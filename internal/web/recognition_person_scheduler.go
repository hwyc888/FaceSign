package web

import (
	"image"
	"time"

	"github.com/hwyc888/FaceSign/internal/person"
)

const personDetectionInterval = 500 * time.Millisecond

type personDetectionCacheEntry struct {
	at         time.Time
	detections []person.Detection
}

func (s *Server) personDetectionsForRecognition(sessionID string, img image.Image, now time.Time) ([]person.Detection, error) {
	if sessionID == "" {
		sessionID = "default"
	}

	s.personDetectionMu.Lock()
	cached, ok := s.personDetectionCache[sessionID]
	if ok && now.Sub(cached.at) >= 0 && now.Sub(cached.at) < personDetectionInterval {
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
		at:         now,
		detections: clonePersonDetections(detections),
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
