package face

import (
	"errors"
	"image"
	"math"
)

var (
	ErrNoFace        = errors.New("no face detected")
	ErrMultipleFaces = errors.New("multiple faces detected")
)

type Detection struct {
	Rectangle image.Rectangle
	Landmarks [5]image.Point
	Score     float32
	Feature   []float32
}

type Engine struct {
	backend string
	by      *byRuntime
}

// Close releases the single active SCRFD/ArcFace runtime.
func (e *Engine) Close() {
	if e == nil {
		return
	}
	e.closeBy()
}

func (e *Engine) Extract(img image.Image, threshold float64) ([]float32, error) {
	detected, err := e.extractDetection(img, threshold, false)
	if err != nil {
		return nil, err
	}
	return detected.Feature, nil
}

func (e *Engine) DetectAll(img image.Image, threshold float64) ([]Detection, error) {
	return e.detectSCRFD(img, threshold)
}

func (e *Engine) Feature(img image.Image, detected Detection) ([]float32, error) {
	return e.featureArcFace(img, detected)
}

func (e *Engine) ExtractAll(img image.Image, threshold float64) ([]Detection, error) {
	detections, err := e.DetectAll(img, threshold)
	if err != nil {
		return nil, err
	}
	for i := range detections {
		feature, err := e.Feature(img, detections[i])
		if err != nil {
			return nil, err
		}
		detections[i].Feature = feature
	}
	return detections, nil
}

func (e *Engine) ExtractEnrollment(img image.Image, threshold float64) ([]float32, error) {
	detected, err := e.extractDetection(img, threshold, true)
	if err != nil {
		return nil, err
	}
	return detected.Feature, nil
}

func (e *Engine) ExtractPhotoSample(img image.Image, threshold float64) (Detection, error) {
	return e.extractDetection(img, threshold, true)
}

func (e *Engine) extractDetection(img image.Image, threshold float64, enrollment bool) (Detection, error) {
	detections, err := e.detectSCRFD(img, threshold)
	if err != nil {
		return Detection{}, err
	}
	selected, err := selectEnrollmentFaceDetection(detections, enrollment)
	if err != nil {
		return Detection{}, err
	}
	feature, err := e.featureArcFace(img, selected)
	if err != nil {
		return Detection{}, err
	}
	selected.Feature = feature
	return selected, nil
}

func selectEnrollmentFaceDetection(faces []Detection, enrollment bool) (Detection, error) {
	if len(faces) == 0 {
		return Detection{}, ErrNoFace
	}
	if len(faces) == 1 {
		return faces[0], nil
	}
	if !enrollment {
		return Detection{}, ErrMultipleFaces
	}
	best := faces[0]
	bestArea := rectangleArea(best.Rectangle)
	for _, candidate := range faces[1:] {
		if area := rectangleArea(candidate.Rectangle); area > bestArea {
			best, bestArea = candidate, area
		}
	}
	if bestArea <= 0 {
		return Detection{}, ErrNoFace
	}
	for _, candidate := range faces {
		if candidate.Rectangle == best.Rectangle {
			continue
		}
		if rectangleArea(candidate.Rectangle) >= bestArea*0.30 {
			return Detection{}, ErrMultipleFaces
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
	var dot, normA, normB float64
	for i := range a {
		x, y := float64(a[i]), float64(b[i])
		dot += x * y
		normA += x * x
		normB += y * y
	}
	if normA <= 0 || normB <= 0 {
		return 0
	}
	cosine := dot / math.Sqrt(normA*normB)
	return math.Max(0, math.Min(1, (cosine+1)/2))
}

func (e *Engine) ModelVersion() string { return "arcface-r50-w600k-v1" }
func (e *Engine) Backend() string      { return "scrfd-arcface" }
