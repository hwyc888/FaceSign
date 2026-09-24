package web

import (
	"image"
	"testing"
	"time"

	"github.com/hwyc888/FaceSign/internal/person"
)

func TestPersonDetectionCacheReusesRecentDetections(t *testing.T) {
	now := time.Now()
	s := &Server{
		personDetectionCache: map[string]personDetectionCacheEntry{
			"session": {
				at: now,
				detections: []person.Detection{{Rectangle: image.Rect(10, 10, 100, 200), Score: 0.9}},
			},
		},
	}

	got, err := s.personDetectionsForRecognition("session", image.NewRGBA(image.Rect(0, 0, 320, 240)), now.Add(200*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Rectangle != image.Rect(10, 10, 100, 200) {
		t.Fatalf("recent person detections were not reused: %#v", got)
	}
	got[0].Rectangle = image.Rect(0, 0, 1, 1)
	cached := s.personDetectionCache["session"].detections[0].Rectangle
	if cached == got[0].Rectangle {
		t.Fatal("caller mutated cached person detection")
	}
}

func TestPersonDetectionCacheExpiresAtDetectionInterval(t *testing.T) {
	now := time.Now()
	s := &Server{
		personDetectionCache: map[string]personDetectionCacheEntry{
			"session": {
				at: now.Add(-personDetectionInterval),
				detections: []person.Detection{{Rectangle: image.Rect(10, 10, 100, 200), Score: 0.9}},
			},
		},
	}
	got, err := s.personDetectionsForRecognition("session", image.NewRGBA(image.Rect(0, 0, 320, 240)), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("stale person detections should not be reused without detector, got %#v", got)
	}
}
