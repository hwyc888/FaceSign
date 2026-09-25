package web

import (
	"image"
	"sort"
	"time"

	"github.com/hwyc888/FaceSign/internal/person"
)

const (
	personDetectionMaxAge = 1800 * time.Millisecond
	personDetectionThreshold = 0.32
	smallPersonDetectionThreshold = 0.22

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

func (s *Server) personDetectionsForRecognition(sessionID string, img image.Image, now time.Time, loadLevel string, recoverSmallPeople bool) ([]person.Detection, error) {
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
	detections, err := s.personEngine.Detect(img, personDetectionThreshold)
	if err != nil {
		return nil, err
	}
	if len(detections) == 0 && recoverSmallPeople {
		detections, err = s.detectSmallPeopleInTiles(img)
		if err != nil {
			return nil, err
		}
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

func (s *Server) detectSmallPeopleInTiles(img image.Image) ([]person.Detection, error) {
	if s.personEngine == nil || img == nil {
		return nil, nil
	}
	var recovered []person.Detection
	for _, tile := range smallPersonDetectionTiles(img.Bounds()) {
		cropped := cropImage(img, tile)
		if cropped == nil {
			continue
		}
		detections, err := s.personEngine.Detect(cropped, smallPersonDetectionThreshold)
		if err != nil {
			return nil, err
		}
		for _, detected := range detections {
			detected.Rectangle = detected.Rectangle.Add(tile.Min).Intersect(img.Bounds())
			if !detected.Rectangle.Empty() {
				recovered = append(recovered, detected)
			}
		}
	}
	return mergePersonDetections(recovered), nil
}

func smallPersonDetectionTiles(bounds image.Rectangle) []image.Rectangle {
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 || width == height {
		return nil
	}
	if width > height {
		tile := height
		return []image.Rectangle{
			image.Rect(bounds.Min.X, bounds.Min.Y, bounds.Min.X+tile, bounds.Max.Y),
			image.Rect(bounds.Max.X-tile, bounds.Min.Y, bounds.Max.X, bounds.Max.Y),
		}
	}
	tile := width
	return []image.Rectangle{
		image.Rect(bounds.Min.X, bounds.Min.Y, bounds.Max.X, bounds.Min.Y+tile),
		image.Rect(bounds.Min.X, bounds.Max.Y-tile, bounds.Max.X, bounds.Max.Y),
	}
}

func mergePersonDetections(items []person.Detection) []person.Detection {
	if len(items) <= 1 {
		return items
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Score > items[j].Score })
	out := make([]person.Detection, 0, len(items))
	for _, candidate := range items {
		duplicate := false
		for _, selected := range out {
			if rectangleIoU(candidate.Rectangle, selected.Rectangle) > 0.45 {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		out = append(out, candidate)
		if len(out) >= 24 {
			break
		}
	}
	return out
}

func clonePersonDetections(src []person.Detection) []person.Detection {
	if len(src) == 0 {
		return nil
	}
	return append([]person.Detection(nil), src...)
}
