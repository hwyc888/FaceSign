package liveness

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hwyc888/FaceSign/internal/inference"

	ort "github.com/yalue/onnxruntime_go"
	xdraw "golang.org/x/image/draw"
)

const inputSize = 128

type modelKind uint8

const (
	modelMN3 modelKind = iota
	modelMiniFASV2SE
)

func modelKindForPath(path string) modelKind {
	if strings.EqualFold(filepath.Base(path), "minifasnet-v2se.onnx") {
		return modelMiniFASV2SE
	}
	return modelMN3
}

func (e *Engine) ModelName() string {
	if e == nil { return "未加载" }
	if e.kind == modelMiniFASV2SE { return "MiniFASNetV2-SE" }
	return "anti-spoof-mn3"
}

var ErrInvalidFace = errors.New("invalid face crop")

type engineWorker struct {
	input   *ort.Tensor[float32]
	output  *ort.Tensor[float32]
	session *ort.AdvancedSession
}

type Engine struct {
	kind      modelKind
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

	kind := modelKindForPath(modelPath)
	engine := &Engine{kind: kind, available: make(chan *engineWorker, engineWorkerCount)}
	for i := 0; i < engineWorkerCount; i++ {
		worker, err := newEngineWorker(modelPath, kind)
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

func newEngineWorker(modelPath string, kind modelKind) (*engineWorker, error) {
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
	inputName, outputName := "actual_input_1", "output1"
	if kind == modelMiniFASV2SE {
		inputName, outputName = "input", "output"
	}
	session, err := ort.NewAdvancedSession(
		modelPath,
		[]string{inputName},
		[]string{outputName},
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

func (e *Engine) acquire(ctx context.Context) (*engineWorker, func(), error) {
	e.mu.RLock()
	if e.closed || e.available == nil {
		e.mu.RUnlock()
		return nil, nil, errors.New("anti-spoof engine is closed")
	}
	select {
	case worker := <-e.available:
		return worker, func() {
			e.available <- worker
			e.mu.RUnlock()
		}, nil
	case <-ctx.Done():
		e.mu.RUnlock()
		return nil, nil, fmt.Errorf("wait for anti-spoof worker: %w", ctx.Err())
	}
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
	if faceRect.Intersect(img.Bounds()).Empty() { return 0, ErrInvalidFace }
	crop := expandedFaceRect(faceRect, img.Bounds())
	if e.kind == modelMN3 && crop.Empty() { return 0, ErrInvalidFace }

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	worker, release, err := e.acquire(ctx)
	if err != nil {
		return 0, err
	}
	defer release()
	releaseInference, err := inference.AcquireContext(ctx)
	if err != nil {
		return 0, fmt.Errorf("wait for CPU inference slot: %w", err)
	}
	defer releaseInference()

	if e.kind == modelMiniFASV2SE {
		// The MiniFAS 1.5x reflected square RGB/255 preprocessing differs from MN3.
		fillMiniFASInput(worker.input.GetData(), resizeMiniFASFaceCrop(img, faceRect))
	} else {
		fillInput(worker.input.GetData(), resizeFaceCrop(img, crop))
	}
	if err := worker.session.Run(); err != nil {
		return 0, fmt.Errorf("run anti-spoof model: %w", err)
	}
	data := worker.output.GetData()
	if len(data) < 2 {
		return 0, errors.New("anti-spoof model returned invalid output")
	}
	return realProbability(float64(data[0]), float64(data[1])), nil
}

// MiniFASNetV2-SE: square face crop at 1.5x, reflected borders and RGB/255.
func resizeMiniFASFaceCrop(img image.Image, faceRect image.Rectangle) *image.RGBA {
	bounds := img.Bounds()
	side := int(math.Max(float64(faceRect.Dx()), float64(faceRect.Dy())) * 1.5)
	if side < 1 { side = 1 }
	cx := float64(faceRect.Min.X+faceRect.Max.X) / 2
	cy := float64(faceRect.Min.Y+faceRect.Max.Y) / 2
	x0 := int(math.Floor(cx - float64(side)/2))
	y0 := int(math.Floor(cy - float64(side)/2))
	crop := image.NewRGBA(image.Rect(0, 0, side, side))
	for y := 0; y < side; y++ {
		sy := bounds.Min.Y + reflect101(y0+y-bounds.Min.Y, bounds.Dy())
		for x := 0; x < side; x++ {
			sx := bounds.Min.X + reflect101(x0+x-bounds.Min.X, bounds.Dx())
			r, g, b, _ := img.At(sx, sy).RGBA()
			crop.SetRGBA(x, y, color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 255})
		}
	}
	out := image.NewRGBA(image.Rect(0, 0, inputSize, inputSize))
	xdraw.CatmullRom.Scale(out, out.Bounds(), crop, crop.Bounds(), xdraw.Src, nil)
	return out
}

func reflect101(pos, n int) int {
	if n <= 1 { return 0 }
	for pos < 0 || pos >= n {
		if pos < 0 { pos = -pos } else { pos = 2*n - 2 - pos }
	}
	return pos
}

func fillMiniFASInput(dst []float32, img *image.RGBA) {
	const plane = inputSize * inputSize
	for y := 0; y < inputSize; y++ {
		for x := 0; x < inputSize; x++ {
			px := img.RGBAAt(x, y)
			i := y*inputSize + x
			dst[i] = float32(px.R) / 255
			dst[plane+i] = float32(px.G) / 255
			dst[2*plane+i] = float32(px.B) / 255
		}
	}
}

func resizeFaceCrop(img image.Image, crop image.Rectangle) *image.RGBA {
	resized := image.NewRGBA(image.Rect(0, 0, inputSize, inputSize))
	// Match the upstream anti-spoof demo's cv.resize default (INTER_LINEAR).
	// Cubic interpolation can add ringing/high-frequency texture that changes
	// the classifier score, especially for small faces from multi-person frames.
	xdraw.BiLinear.Scale(resized, resized.Bounds(), img, crop, xdraw.Src, nil)
	return resized
}

func expandedFaceRect(rect, bounds image.Rectangle) image.Rectangle {
	if rect.Empty() || bounds.Empty() {
		return image.Rectangle{}
	}
	cx := float64(rect.Min.X+rect.Max.X) / 2
	cy := float64(rect.Min.Y+rect.Max.Y) / 2
	w := float64(rect.Dx()) * 1.10
	h := float64(rect.Dy()) * 1.05
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
