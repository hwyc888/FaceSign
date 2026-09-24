package web

import (
	"image"
	"time"

	"github.com/hwyc888/FaceSign/internal/person"
)

const (
	personDetectionMaxAge = 1800 * time.Millisecond

	recognitionLoadNormal  = "normal"
	recognitionLoadReduced = "reduced"
	recognitionLoadProtect = "protect"
)

type personDetectionCacheEntry struct {
	at             time.Time
	detections     []person.Detection
	reuseRemaining int
}

func normalizeRecognitionLoadLevel(value string) string {
	switch value {
	case recognitionLoadReduced:
		return recognitionLoadReduced
	case recognitionLoadProtect:
		return recognitionLoadProtect
	default:
		return recognitionLoadNormal
	}
}

func personDetectionReuseFramesForLoad(loadLevel string) int {
	switch normalizeRecognitionLoadLevel(loadLevel) {
	case recognitionLoadProtect:
		return 4
	case recognitionLoadReduced:
		return 3
	default:
		return 2
	}
}

func (s *Server) personDetectionsForRecognition(sessionID string, img image.Image, now time.Time, loadLevel string) ([]person.Detection, error) {
	if sessionID == "" {
		sessionID = "default"
	}

	reuseFrames := personDetectionReuseFramesForLoad(loadLevel)
	s.personDetectionMu.Lock()
	cached, ok := s.personDetectionCache[sessionID]
	age := now.Sub(cached.at)
	if ok && cached.reuseRemaining > reuseFrames {
		cached.reuseRemaining = reuseFrames
	}
	if ok && age >= 0 && age < personDetectionMaxAge && cached.reuseRemaining > 0 {
		cached.reuseRemaining--
		s.personDetectionCache[sessionID] = cached
		out := clonePersonDetections(cached.detections)
		s.personDetectionMu.Unlock()
		return out, nil
	}
	s.personDetectionMu.Unlock()

	if s.personEngine == nil {
		return nil, nil
	}
	detections, err := s.personEngine.Detect(img, 0.32)
	if err != nil {
		return nil, err
	}

	s.personDetectionMu.Lock()
	if s.personDetectionCache == nil {
		s.personDetectionCache = make(map[string]personDetectionCacheEntry)
	}
	for key, entry := range s.personDetectionCache {
		if now.Sub(entry.at) > 5*time.Second {
			delete(s.personDetectionCache, key)
		}
	}
	s.personDetectionCache[sessionID] = personDetectionCacheEntry{
		at:             now,
		detections:     clonePersonDetections(detections),
		reuseRemaining: reuseFrames,
	}
	s.personDetectionMu.Unlock()
	return detections, nil
}

func clonePersonDetections(src []person.Detection) []person.Detection {
	if len(src) == 0 {
		return nil
	}
	return append([]person.Detection(nil), src...)
}
