package person

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"sort"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
	xdraw "golang.org/x/image/draw"
)

const (
	inputSize  = 416
	outputRows = 3549
	outputCols = 85
	personClass = 0
)

type Detection struct {
	Rectangle image.Rectangle
	Score     float64
}

type Engine struct {
	mu      sync.Mutex
	input   *ort.Tensor[float32]
	output  *ort.Tensor[float32]
	session *ort.AdvancedSession
}

func New(modelPath string) (*Engine, error) {
	info, err := os.Stat(modelPath)
	if err != nil || info.IsDir() {
		return nil, fmt.Errorf("person detector model not found: %s", modelPath)
	}
	if !ort.IsInitialized() {
		return nil, errors.New("ONNX Runtime environment is not initialized")
	}

	input, err := ort.NewEmptyTensor[float32](ort.NewShape(1, 3, inputSize, inputSize))
	if err != nil {
		return nil, fmt.Errorf("create person detector input tensor: %w", err)
	}
	output, err := ort.NewEmptyTensor[float32](ort.NewShape(1, outputRows, outputCols))
	if err != nil {
		input.Destroy()
		return nil, fmt.Errorf("create person detector output tensor: %w", err)
	}
	session, err := ort.NewAdvancedSession(
		modelPath,
		[]string{"images"},
		[]string{"output"},
		[]ort.Value{input},
		[]ort.Value{output},
		nil,
	)
	if err != nil {
		output.Destroy()
		input.Destroy()
		return nil, fmt.Errorf("load YOLOX person detector: %w", err)
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

func (e *Engine) Detect(img image.Image, threshold float64) ([]Detection, error) {
	if img == nil || img.Bounds().Dx() < 40 || img.Bounds().Dy() < 40 {
		return nil, nil
	}
	if threshold <= 0 {
		threshold = 0.35
	}
	if threshold < 0.10 {
		threshold = 0.10
	}
	if threshold > 0.95 {
		threshold = 0.95
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.session == nil || e.input == nil || e.output == nil {
		return nil, errors.New("person detector is closed")
	}

	ratio := fillYOLOXInput(e.input.GetData(), img)
	if ratio <= 0 {
		return nil, nil
	}
	if err := e.session.Run(); err != nil {
		return nil, fmt.Errorf("run YOLOX person detector: %w", err)
	}
	return decodePersonOutput(e.output.GetData(), ratio, img.Bounds(), threshold), nil
}

func fillYOLOXInput(dst []float32, img image.Image) float64 {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 || len(dst) < 3*inputSize*inputSize {
		return 0
	}
	ratio := math.Min(float64(inputSize)/float64(width), float64(inputSize)/float64(height))
	scaledWidth := int(math.Round(float64(width) * ratio))
	scaledHeight := int(math.Round(float64(height) * ratio))
	if scaledWidth < 1 || scaledHeight < 1 {
		return 0
	}
	if scaledWidth > inputSize {
		scaledWidth = inputSize
	}
	if scaledHeight > inputSize {
		scaledHeight = inputSize
	}

	canvas := image.NewRGBA(image.Rect(0, 0, inputSize, inputSize))
	for y := 0; y < inputSize; y++ {
		for x := 0; x < inputSize; x++ {
			canvas.SetRGBA(x, y, color.RGBA{R: 114, G: 114, B: 114, A: 255})
		}
	}
	xdraw.BiLinear.Scale(
		canvas,
		image.Rect(0, 0, scaledWidth, scaledHeight),
		img,
		bounds,
		xdraw.Src,
		nil,
	)

	const plane = inputSize * inputSize
	for y := 0; y < inputSize; y++ {
		for x := 0; x < inputSize; x++ {
			r16, g16, b16, _ := canvas.At(x, y).RGBA()
			i := y*inputSize + x
			dst[i] = float32(b16) / 257.0
			dst[plane+i] = float32(g16) / 257.0
			dst[2*plane+i] = float32(r16) / 257.0
		}
	}
	return ratio
}

func decodePersonOutput(data []float32, ratio float64, bounds image.Rectangle, threshold float64) []Detection {
	if ratio <= 0 || len(data) < outputRows*outputCols {
		return nil
	}
	strides := []int{8, 16, 32}
	candidates := make([]Detection, 0, 16)
	row := 0
	for _, stride := range strides {
		gridSize := inputSize / stride
		for gy := 0; gy < gridSize; gy++ {
			for gx := 0; gx < gridSize; gx++ {
				base := row * outputCols
				row++
				score := float64(data[base+4] * data[base+5+personClass])
				if score < threshold {
					continue
				}
				cx := (float64(data[base]) + float64(gx)) * float64(stride)
				cy := (float64(data[base+1]) + float64(gy)) * float64(stride)
				w := math.Exp(float64(data[base+2])) * float64(stride)
				h := math.Exp(float64(data[base+3])) * float64(stride)
				rect := image.Rect(
					int(math.Round((cx-w/2)/ratio))+bounds.Min.X,
					int(math.Round((cy-h/2)/ratio))+bounds.Min.Y,
					int(math.Round((cx+w/2)/ratio))+bounds.Min.X,
					int(math.Round((cy+h/2)/ratio))+bounds.Min.Y,
				).Intersect(bounds)
				if rect.Dx() < 16 || rect.Dy() < 24 {
					continue
				}
				candidates = append(candidates, Detection{Rectangle: rect, Score: score})
			}
		}
	}
	return nonMaximumSuppression(candidates, 0.45, 24)
}

func nonMaximumSuppression(candidates []Detection, threshold float64, maxResults int) []Detection {
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Score > candidates[j].Score })
	out := make([]Detection, 0, len(candidates))
	for _, candidate := range candidates {
		keep := true
		for _, selected := range out {
			if intersectionOverUnion(candidate.Rectangle, selected.Rectangle) > threshold {
				keep = false
				break
			}
		}
		if !keep {
			continue
		}
		out = append(out, candidate)
		if maxResults > 0 && len(out) >= maxResults {
			break
		}
	}
	return out
}

func intersectionOverUnion(a, b image.Rectangle) float64 {
	intersection := a.Intersect(b)
	if intersection.Empty() {
		return 0
	}
	intersectionArea := float64(intersection.Dx() * intersection.Dy())
	unionArea := float64(a.Dx()*a.Dy()+b.Dx()*b.Dy()) - intersectionArea
	if unionArea <= 0 {
		return 0
	}
	return intersectionArea / unionArea
}
