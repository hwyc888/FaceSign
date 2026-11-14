package face

import (
	"errors"
	"fmt"
	"image"
	"math"
	"os"
	"sync"

	"github.com/hwyc888/FaceSign/internal/inference"

	onnxface "github.com/leandroveronezi/go-onnxface"
	"github.com/leandroveronezi/go-onnxface/sface"
	"github.com/leandroveronezi/go-onnxface/yunet"
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

type engineWorker struct {
	detector   *yunet.Detector
	recognizer *sface.Recognizer
}

type Engine struct {
	mu        sync.RWMutex
	workers   []*engineWorker
	available chan *engineWorker
	closed    bool
}

const engineWorkerCount = inference.MaxConcurrent

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

	engine := &Engine{available: make(chan *engineWorker, engineWorkerCount)}
	cleanup := func() {
		for _, worker := range engine.workers {
			worker.close()
		}
		_ = onnxface.CloseEnvironment()
	}
	for i := 0; i < engineWorkerCount; i++ {
		detector, err := yunet.NewDetector(detectorModel)
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("load YuNet worker %d: %w", i+1, err)
		}
		recognizer, err := sface.NewRecognizer(recognizerModel)
		if err != nil {
			detector.Close()
			cleanup()
			return nil, fmt.Errorf("load SFace worker %d: %w", i+1, err)
		}
		worker := &engineWorker{detector: detector, recognizer: recognizer}
		engine.workers = append(engine.workers, worker)
		engine.available <- worker
	}
	return engine, nil
}

func (w *engineWorker) close() {
	if w == nil {
		return
	}
	if w.recognizer != nil {
		w.recognizer.Close()
		w.recognizer = nil
	}
	if w.detector != nil {
		w.detector.Close()
		w.detector = nil
	}
}

func (e *Engine) acquire() (*engineWorker, func(), error) {
	e.mu.RLock()
	if e.closed || e.available == nil {
		e.mu.RUnlock()
		return nil, nil, errors.New("face engine is closed")
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
	_ = onnxface.CloseEnvironment()
}

func (e *Engine) Extract(img image.Image, threshold float64) ([]float32, error) {
	detected, err := e.extractDetection(img, threshold, false)
	if err != nil {
		return nil, err
	}
	return detected.Feature, nil
}

// DetectAll returns face geometry without running SFace. Recognition uses
// this first so low-quality frames can be tracked without wasting embedding
// inference, then extracts a feature only from the best frame for each track.
func (e *Engine) DetectAll(img image.Image, threshold float64) ([]Detection, error) {
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

	worker, release, err := e.acquire()
	if err != nil {
		return nil, err
	}
	defer release()
	releaseInference := inference.Acquire()
	defer releaseInference()

	worker.detector.ScoreThreshold = float32(threshold)
	faces, err := worker.detector.Detect(img)
	if err != nil {
		return nil, fmt.Errorf("detect face: %w", err)
	}
	if len(faces) == 0 {
		return nil, ErrNoFace
	}

	out := make([]Detection, 0, len(faces))
	for _, detected := range faces {
		out = append(out, Detection{
			Rectangle: detected.Rectangle,
			Landmarks: detected.Landmarks,
			Score:     detected.Score,
		})
	}
	return out, nil
}

// func (e *Engine) Feature(img image.Image, detected Detection) ([]float32, error) {
	if img == nil || detected.Rectangle.Empty() {
		return nil, ErrNoFace
	}
	worker, release, err := e.acquire()
	if err != nil {
		return nil, err
	}
	defer release()
	releaseInference := inference.Acquire()
	defer releaseInference()

	aligned := worker.recognizer.Align(img, detected.Landmarks)
	feature, err := worker.recognizer.Feature(aligned)
	if err != nil {
		return nil, fmt.Errorf("extract face feature: %w", err)
	}
	return append([]float32(nil), feature...), nil
}

// func (e *Engine) ExtractAll(img image.Image, threshold float64) ([]Detection, error) {
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

// ExtractEnrollment is slightly more tolerant than recognition. A clearly
// dominant foreground face may be enrolled even when YuNet also finds a much
// smaller background face. Two similarly sized faces are still rejected.
func (e *Engine) ExtractEnrollment(img image.Image, threshold float64) ([]float32, error) {
	detected, err := e.extractDetection(img, threshold, true)
	if err != nil {
		return nil, err
	}
	return detected.Feature, nil
}

// ExtractPhotoSample returns the selected enrollment face together with its
// geometry and detector score so bulk imports can reject weak portrait photos.
func (e *Engine) ExtractPhotoSample(img image.Image, threshold float64) (Detection, error) {
	return e.extractDetection(img, threshold, true)
}

func (e *Engine) extractDetection(img image.Image, threshold float64, enrollment bool) (Detection, error) {
	if img == nil || img.Bounds().Dx() < 40 || img.Bounds().Dy() < 40 {
		return Detection{}, ErrNoFace
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

	worker, release, err := e.acquire()
	if err != nil {
		return Detection{}, err
	}
	defer release()
	releaseInference := inference.Acquire()
	defer releaseInference()

	worker.detector.ScoreThreshold = float32(threshold)
	faces, err := worker.detector.Detect(img)
	if err != nil {
		return Detection{}, fmt.Errorf("detect face: %w", err)
	}
	if len(faces) == 0 {
		return Detection{}, ErrNoFace
	}

	selected := faces[0]
	if len(faces) > 1 {
		if !enrollment {
			return Detection{}, ErrMultipleFaces
		}
		selected, err = selectEnrollmentFace(faces)
		if err != nil {
			return Detection{}, err
		}
	}

	aligned := worker.recognizer.Align(img, selected.Landmarks)
	feature, err := worker.recognizer.Feature(aligned)
	if err != nil {
		return Detection{}, fmt.Errorf("extract face feature: %w", err)
	}
	return Detection{
		Rectangle: selected.Rectangle,
		Landmarks: selected.Landmarks,
		Score:     selected.Score,
		Feature:   append([]float32(nil), feature...),
	}, nil
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
