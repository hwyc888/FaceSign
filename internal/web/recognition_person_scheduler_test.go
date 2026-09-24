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
				reuseRemaining: personDetectionReuseFrames,
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

func TestPersonDetectionCacheExpiresAtMaxAge(t *testing.T) {
	now := time.Now()
	s := &Server{
		personDetectionCache: map[string]personDetectionCacheEntry{
			"session": {
				at: now.Add(-personDetectionMaxAge),
				detections: []person.Detection{{Rectangle: image.Rect(10, 10, 100, 200), Score: 0.9}},
				reuseRemaining: personDetectionReuseFrames,
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

func TestPersonDetectionCacheReusesOnlyTwoRecognitionFrames(t *testing.T) {
	now := time.Now()
	s := &Server{
		personDetectionCache: map[string]personDetectionCacheEntry{
			"session": {
				at: now,
				detections: []person.Detection{{Rectangle: image.Rect(10, 10, 100, 200), Score: 0.9}},
				reuseRemaining: personDetectionReuseFrames,
			},
		},
	}
	img := image.NewRGBA(image.Rect(0, 0, 320, 240))
	for i := 0; i < personDetectionReuseFrames; i++ {
		got, err := s.personDetectionsForRecognition("session", img, now.Add(time.Duration(i+1)*100*time.Millisecond))
		if err != nil || len(got) != 1 {
			t.Fatalf("reuse %d failed: detections=%d err=%v", i+1, len(got), err)
		}
	}
	got, err := s.personDetectionsForRecognition("session", img, now.Add(400*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("third reuse must require a fresh YOLOX detection, got %#v", got)
	}
}
