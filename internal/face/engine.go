package face

import (
	"errors"
	"fmt"
	"image"
	"math"
	"os"
	"sync"

	onnxface "github.com/leandroveronezi/go-onnxface"
	"github.com/leandroveronezi/go-onnxface/sface"
	"github.com/leandroveronezi/go-onnxface/yunet"
)

var (
	ErrNoFace        = errors.New("no face detected")
	ErrMultipleFaces = errors.New("multiple faces detected")
)

type Engine struct {
	mu         sync.Mutex
	detector   *yunet.Detector
	recognizer *sface.Recognizer
}

func New(runtimePath, detectorModel, recognizerModel string) (*Engine, error) {
	for label, path := range map[string]string{
		"ONNX Runtime": runtimePath,
		"YuNet model":  detectorModel,
		"SFace model":  recognizerModel,
	} {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			return nil, fmt.Errorf("%s not found: %s", label, path)
		}
	}
	if err := onnxface.InitEnvironment(runtimePath); err != nil {
		return nil, fmt.Errorf("initialize ONNX Runtime: %w", err)
	}
	detector, err := yunet.NewDetector(detectorModel)
	if err != nil {
		_ = onnxface.CloseEnvironment()
		return nil, fmt.Errorf("load YuNet model: %w", err)
	}
	recognizer, err := sface.NewRecognizer(recognizerModel)
	if err != nil {
		detector.Close()
		_ = onnxface.CloseEnvironment()
		return nil, fmt.Errorf("load SFace model: %w", err)
	}
	return &Engine{detector: detector, recognizer: recognizer}, nil
}

func (e *Engine) Close() {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.recognizer != nil {
		e.recognizer.Close()
		e.recognizer = nil
	}
	if e.detector != nil {
		e.detector.Close()
		e.detector = nil
	}
	_ = onnxface.CloseEnvironment()
}

func (e *Engine) Extract(img image.Image, threshold float64) ([]float32, error) {
	return e.extract(img, threshold, false)
}

// ExtractEnrollment is slightly more tolerant than recognition. A clearly
// dominant foreground face may be enrolled even when YuNet also finds a much
// smaller background face. Two similarly sized faces are still rejected.
func (e *Engine) ExtractEnrollment(img image.Image, threshold float64) ([]float32, error) {
	return e.extract(img, threshold, true)
}

func (e *Engine) extract(img image.Image, threshold float64, enrollment bool) ([]float32, error) {
	if img == nil || img.Bounds().Dx() < 40 || img.Bounds().Dy() < 40 {
		return nil, ErrNoFace
	}
	if threshold <= 0 {
		threshold = 0.80
	}
	if threshold < 0.10 {
		threshold = 0.10
	}
	if threshold > 0.99 {
		threshold = 0.99
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.detector.ScoreThreshold = float32(threshold)
	faces, err := e.detector.Detect(img)
	if err != nil {
		return nil, fmt.Errorf("detect face: %w", err)
	}
	if len(faces) == 0 {
		return nil, ErrNoFace
	}

	selected := faces[0]
	if len(faces) > 1 {
		if !enrollment {
			return nil, ErrMultipleFaces
		}
		selected, err = selectEnrollmentFace(faces)
		if err != nil {
			return nil, err
		}
	}

	aligned := e.recognizer.Align(img, selected.Landmarks)
	feature, err := e.recognizer.Feature(aligned)
	if err != nil {
		return nil, fmt.Errorf("extract face feature: %w", err)
	}
	return append([]float32(nil), feature...), nil
}

func selectEnrollmentFace(faces []onnxface.Face) (onnxface.Face, error) {
	if len(faces) == 0 {
		return onnxface.Face{}, ErrNoFace
	}

	best := faces[0]
	bestArea := rectangleArea(best.Rectangle)
	for _, candidate := range faces[1:] {
		area := rectangleArea(candidate.Rectangle)
		if area > bestArea {
			best = candidate
			bestArea = area
		}
	}
	if bestArea <= 0 {
		return onnxface.Face{}, ErrNoFace
	}

	const significantFaceRatio = 0.30
	for _, candidate := range faces {
		if candidate.Rectangle == best.Rectangle {
			continue
		}
		if rectangleArea(candidate.Rectangle) >= bestArea*significantFaceRatio {
			return onnxface.Face{}, ErrMultipleFaces
		}
	}
	return best, nil
}

func rectangleArea(rect image.Rectangle) float64 {
	if rect.Empty() {
		return 0
	}
	return float64(rect.Dx() * rect.Dy())
}

func Similarity(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	cosine := onnxface.Match(a, b, onnxface.DistanceCosine)
	return math.Max(0, math.Min(1, (cosine+1)/2))
}

func RuntimeVersion() string {
	return onnxface.Version()
}
