package liveness

import (
	"errors"
	"fmt"
	"image"
	"math"
	"os"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
	xdraw "golang.org/x/image/draw"
)

const inputSize = 128

var ErrInvalidFace = errors.New("invalid face crop")

type Engine struct {
	mu      sync.Mutex
	input   *ort.Tensor[float32]
	output  *ort.Tensor[float32]
	session *ort.AdvancedSession
}

func New(modelPath string) (*Engine, error) {
	info, err := os.Stat(modelPath)
	if err != nil || info.IsDir() {
		return nil, fmt.Errorf("anti-spoof model not found: %s", modelPath)
	}
	if !ort.IsInitialized() {
		return nil, errors.New("ONNX Runtime environment is not initialized")
	}

	input, err := ort.NewEmptyTensor[float32](ort.NewShape(1, 3, inputSize, inputSize))
	if err != nil {
		return nil, fmt.Errorf("create anti-spoof input tensor: %w", err)
	}
	output, err := ort.NewEmptyTensor[float32](ort.NewShape(1, 2))
	if err != nil {
		input.Destroy()
		return nil, fmt.Errorf("create anti-spoof output tensor: %w", err)
	}
	session, err := ort.NewAdvancedSession(
		modelPath,
		[]string{"actual_input_1"},
		[]string{"output1"},
		[]ort.Value{input},
		[]ort.Value{output},
		nil,
	)
	if err != nil {
		output.Destroy()
		input.Destroy()
		return nil, fmt.Errorf("load anti-spoof model: %w", err)
	}
	return &Engine{input: input, output: output, session: session}, nil
}

func (e *Engine) Close() {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.session != nil {
		_ = e.session.Destroy()
		e.session = nil
	}
	if e.output != nil {
		_ = e.output.Destroy()
		e.output = nil
	}
	if e.input != nil {
		_ = e.input.Destroy()
		e.input = nil
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

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.session == nil || e.input == nil || e.output == nil {
		return 0, errors.New("anti-spoof engine is closed")
	}

	resized := image.NewRGBA(image.Rect(0, 0, inputSize, inputSize))
	xdraw.CatmullRom.Scale(resized, resized.Bounds(), img, crop, xdraw.Src, nil)
	fillInput(e.input.GetData(), resized)
	if err := e.session.Run(); err != nil {
		return 0, fmt.Errorf("run anti-spoof model: %w", err)
	}
	data := e.output.GetData()
	if len(data) < 2 {
		return 0, errors.New("anti-spoof model returned invalid output")
	}
	return realProbability(float64(data[0]), float64(data[1])), nil
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
