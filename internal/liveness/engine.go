package liveness

import (
	"errors"
	"fmt"
	"image"
	"math"
	"os"
	"sync"

	"github.com/hwyc888/FaceSign/internal/inference"

	ort "github.com/yalue/onnxruntime_go"
	xdraw "golang.org/x/image/draw"
)

const inputSize = 128

var ErrInvalidFace = errors.New("invalid face crop")

type engineWorker struct {
	input   *ort.Tensor[float32]
	output  *ort.Tensor[float32]
	session *ort.AdvancedSession
}

type Engine struct {
	mu        sync.RWMutex
	workers   []*engineWorker
	available chan *engineWorker
	closed    bool
}

const engineWorkerCount = inference.MaxConcurrent

func New(modelPath string) (*Engine, error) {
	info, err := os.Stat(modelPath)
	if err != nil || info.IsDir() {
		return nil, fmt.Errorf("anti-spoof model not found: %s", modelPath)
	}
	if !ort.IsInitialized() {
		return nil, errors.New("ONNX Runtime environment is not initialized")
	}

	engine := &Engine{available: make(chan *engineWorker, engineWorkerCount)}
	for i := 0; i < engineWorkerCount; i++ {
		worker, err := newEngineWorker(modelPath)
		if err != nil {
			for _, existing := range engine.workers {
				existing.close()
			}
			return nil, fmt.Errorf("load anti-spoof worker %d: %w", i+1, err)
		}
		engine.workers = append(engine.workers, worker)
		engine.available <- worker
	}
	return engine, nil
}

func newEngineWorker(modelPath string) (*engineWorker, error) {
	input, err := ort.NewEmptyTensor[float32](ort.NewShape(1, 3, inputSize, inputSize))
	if err != nil {
		return nil, fmt.Errorf("create anti-spoof input tensor: %w", err)
	}
	output, err := ort.NewEmptyTensor[float32](ort.NewShape(1, 2))
	if err != nil {
		input.Destroy()
		return nil, fmt.Errorf("create anti-spoof output tensor: %w", err)
	}
	options, err := ort.NewSessionOptions()
	if err != nil {
		output.Destroy()
		input.Destroy()
		return nil, fmt.Errorf("create anti-spoof session options: %w", err)
	}
	if err := options.SetIntraOpNumThreads(1); err != nil {
		_ = options.Destroy()
		output.Destroy()
		input.Destroy()
		return nil, fmt.Errorf("configure anti-spoof intra-op threads: %w", err)
	}
	if err := options.SetInterOpNumThreads(1); err != nil {
		_ = options.Destroy()
		output.Destroy()
		input.Destroy()
		return nil, fmt.Errorf("configure anti-spoof inter-op threads: %w", err)
	}
	session, err := ort.NewAdvancedSession(
		modelPath,
		[]string{"actual_input_1"},
		[]string{"output1"},
		[]ort.Value{input},
		[]ort.Value{output},
		options,
	)
	_ = options.Destroy()
	if err != nil {
		output.Destroy()
		input.Destroy()
		return nil, fmt.Errorf("load anti-spoof model: %w", err)
	}
	return &engineWorker{input: input, output: output, session: session}, nil
}

func (w *engineWorker) close() {
	if w == nil {
		return
	}
	if w.session != nil {
		_ = w.session.Destroy()
		w.session = nil
	}
	if w.output != nil {
		_ = w.output.Destroy()
		w.output = nil
	}
	if w.input != nil {
		_ = w.input.Destroy()
		w.input = nil
	}
}

func (e *Engine) acquire() (*engineWorker, func(), error) {
	e.mu.RLock()
	if e.closed || e.available == nil {
		e.mu.RUnlock()
		return nil, nil, errors.New("anti-spoof engine is closed")
	}
	worker := <-e.available
	return worker, func() {
		e.available <- worker
		e.mu.RUnlock()
	}, nil
}

func (e *Engine) Close() {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return
	}
	e.closed = true
	for _, worker := range e.workers {
		worker.close()
	}
}

func (e *Engine) Score(img image.Image, faceRect image.Rectangle) (float64, error) {
	if img == nil || faceRect.Empty() {
		return 0, ErrInvalidFace
	}
	crop := expandedFaceRect(faceRect, img.Bounds())
	if crop.Empty() {
		return 0, ErrInvalidFace
	}

	worker, release, err := e.acquire()
	if err != nil {
		return 0, err
	}
	defer release()
	releaseInference := inference.Acquire()
	defer releaseInference()

	resized := image.NewRGBA(image.Rect(0, 0, inputSize, inputSize))
	xdraw.CatmullRom.Scale(resized, resized.Bounds(), img, crop, xdraw.Src, nil)
	applyLowLightCompensation(resized)
	fillInput(worker.input.GetData(), resized)
	if err := worker.session.Run(); err != nil {
		return 0, fmt.Errorf("run anti-spoof model: %w", err)
	}
	data := worker.output.GetData()
	if len(data) < 2 {
		return 0, errors.New("anti-spoof model returned invalid output")
	}
	return realProbability(float64(data[0]), float64(data[1])), nil
}

func applyLowLightCompensation(img *image.RGBA) float64 {
	if img == nil || img.Bounds().Empty() {
		return 1
	}
	brightness := averageLuminance(img)
	gamma := lowLightGamma(brightness)
	if gamma >= 0.999 {
		return 1
	}

	var lookup [256]uint8
	for i := range lookup {
		lookup[i] = uint8(math.Round(math.Pow(float64(i)/255, gamma) * 255))
	}
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			offset := img.PixOffset(x, y)
			img.Pix[offset] = lookup[img.Pix[offset]]
			img.Pix[offset+1] = lookup[img.Pix[offset+1]]
			img.Pix[offset+2] = lookup[img.Pix[offset+2]]
		}
	}
	return gamma
}

func averageLuminance(img image.Image) float64 {
	if img == nil || img.Bounds().Empty() {
		return 0
	}
	var total float64
	var samples int
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			total += 0.299*float64(r)/257 + 0.587*float64(g)/257 + 0.114*float64(b)/257
			samples++
		}
	}
	if samples == 0 {
		return 0
	}
	return total / float64(samples)
}

func lowLightGamma(brightness float64) float64 {
	if brightness <= 1 || brightness >= 100 {
		return 1
	}
	normalized := brightness / 255
	target := 110.0 / 255
	gamma := math.Log(target) / math.Log(normalized)
	return math.Max(0.55, math.Min(1, gamma))
}

func expandedFaceRect(rect, bounds image.Rectangle) image.Rectangle {
	if rect.Empty() || bounds.Empty() {
		return image.Rectangle{}
	}
	cx := float64(rect.Min.X+rect.Max.X) / 2
	cy := float64(rect.Min.Y+rect.Max.Y) / 2
	w := float64(rect.Dx()) * 1.35
	h := float64(rect.Dy()) * 1.35
	out := image.Rect(
		int(math.Round(cx-w/2)),
		int(math.Round(cy-h/2)),
		int(math.Round(cx+w/2)),
		int(math.Round(cy+h/2)),
	)
	return out.Intersect(bounds)
}

func fillInput(dst []float32, img image.Image) {
	const plane = inputSize * inputSize
	means := [3]float64{151.2405, 119.5950, 107.8395}
	scales := [3]float64{63.0105, 56.4570, 55.0035}

	for y := 0; y < inputSize; y++ {
		for x := 0; x < inputSize; x++ {
			r16, g16, b16, _ := img.At(x, y).RGBA()
			r := float64(r16) / 257.0
			g := float64(g16) / 257.0
			b := float64(b16) / 257.0
			i := y*inputSize + x
			dst[i] = float32((r - means[0]) / scales[0])
			dst[plane+i] = float32((g - means[1]) / scales[1])
			dst[2*plane+i] = float32((b - means[2]) / scales[2])
		}
	}
}

func realProbability(realValue, spoofValue float64) float64 {
	if realValue >= 0 && spoofValue >= 0 {
		sum := realValue + spoofValue
		if sum > 0 && math.Abs(sum-1) < 0.05 {
			return clamp01(realValue / sum)
		}
	}
	maxValue := math.Max(realValue, spoofValue)
	realExp := math.Exp(realValue - maxValue)
	spoofExp := math.Exp(spoofValue - maxValue)
	return clamp01(realExp / (realExp + spoofExp))
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
