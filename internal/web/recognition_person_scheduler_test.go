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
				reuseRemaining: personDetectionReuseFramesForLoad(recognitionLoadNormal),
			},
		},
	}

	got, err := s.personDetectionsForRecognition("session", image.NewRGBA(image.Rect(0, 0, 320, 240)), now.Add(200*time.Millisecond), recognitionLoadNormal, false)
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
				reuseRemaining: personDetectionReuseFramesForLoad(recognitionLoadNormal),
			},
		},
	}
	got, err := s.personDetectionsForRecognition("session", image.NewRGBA(image.Rect(0, 0, 320, 240)), now, recognitionLoadNormal, false)
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
				reuseRemaining: personDetectionReuseFramesForLoad(recognitionLoadNormal),
			},
		},
	}
	img := image.NewRGBA(image.Rect(0, 0, 320, 240))
	for i := 0; i < personDetectionReuseFramesForLoad(recognitionLoadNormal); i++ {
		got, err := s.personDetectionsForRecognition("session", img, now.Add(time.Duration(i+1)*100*time.Millisecond), recognitionLoadNormal, false)
		if err != nil || len(got) != 1 {
			t.Fatalf("reuse %d failed: detections=%d err=%v", i+1, len(got), err)
		}
	}
	got, err := s.personDetectionsForRecognition("session", img, now.Add(400*time.Millisecond), recognitionLoadNormal, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("third reuse must require a fresh YOLOX detection, got %#v", got)
	}
}


func TestPersonDetectionReuseAdaptsToVideoLoad(t *testing.T) {
	cases := []struct {
		level string
		want  int
	}{
		{recognitionLoadNormal, 2},
		{recognitionLoadReduced, 3},
		{recognitionLoadProtect, 4},
		{"invalid", 2},
	}
	for _, tc := range cases {
		if got := personDetectionReuseFramesForLoad(tc.level); got != tc.want {
			t.Fatalf("load %q reuse=%d want=%d", tc.level, got, tc.want)
		}
	}
}


func TestSmallPersonDetectionTilesMagnifyLandscapeTargets(t *testing.T) {
	bounds := image.Rect(0, 0, 1280, 720)
	tiles := smallPersonDetectionTiles(bounds)
	if len(tiles) != 2 {
		t.Fatalf("tiles=%d want=2", len(tiles))
	}
	if tiles[0] != image.Rect(0, 0, 720, 720) || tiles[1] != image.Rect(560, 0, 1280, 720) {
		t.Fatalf("unexpected landscape tiles: %#v", tiles)
	}
	personRect := image.Rect(730, 410, 775, 520)
	if personRect.Intersect(tiles[1]).Empty() {
		t.Fatalf("right-side small person must be covered by a magnified tile: person=%v tiles=%v", personRect, tiles)
	}
}

func TestMergePersonDetectionsRemovesTileOverlapDuplicate(t *testing.T) {
	items := []person.Detection{
		{Rectangle: image.Rect(700, 400, 780, 560), Score: 0.81},
		{Rectangle: image.Rect(706, 406, 784, 562), Score: 0.72},
		{Rectangle: image.Rect(100, 100, 160, 260), Score: 0.70},
	}
	got := mergePersonDetections(items)
	if len(got) != 2 {
		t.Fatalf("merged detections=%d want=2: %#v", len(got), got)
	}
	if got[0].Score != 0.81 {
		t.Fatalf("highest-score duplicate was not preserved: %#v", got)
	}
}
