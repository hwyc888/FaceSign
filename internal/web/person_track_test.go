package web

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/hwyc888/FaceSign/internal/face"
	"github.com/hwyc888/FaceSign/internal/person"
	"github.com/hwyc888/FaceSign/internal/store"
)

func TestPersonTrackerKeepsIDAcrossMotion(t *testing.T) {
	tracker := newPersonTracker()
	now := time.Now()
	first := tracker.Observe("camera", []person.Detection{{
		Rectangle: image.Rect(100, 80, 260, 500), Score: 0.9,
	}}, now)
	second := tracker.Observe("camera", []person.Detection{{
		Rectangle: image.Rect(112, 86, 272, 506), Score: 0.88,
	}}, now.Add(180*time.Millisecond))
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("unexpected tracks: first=%d second=%d", len(first), len(second))
	}
	if first[0].TrackID != second[0].TrackID {
		t.Fatalf("track changed across small motion: %q -> %q", first[0].TrackID, second[0].TrackID)
	}
}

func TestPersonTrackerExpiresAndCreatesNewID(t *testing.T) {
	tracker := newPersonTracker()
	now := time.Now()
	first := tracker.Observe("camera", []person.Detection{{
		Rectangle: image.Rect(100, 80, 260, 500), Score: 0.9,
	}}, now)[0]
	second := tracker.Observe("camera", []person.Detection{{
		Rectangle: image.Rect(100, 80, 260, 500), Score: 0.9,
	}}, now.Add(personTrackTTL+100*time.Millisecond))[0]
	if first.TrackID == second.TrackID {
		t.Fatalf("expired track reused: %q", first.TrackID)
	}
}

func TestPersonTrackerUsesBestFaceAndCachesIdentity(t *testing.T) {
	tracker := newPersonTracker()
	now := time.Now()
	obs := tracker.Observe("camera", []person.Detection{{
		Rectangle: image.Rect(100, 80, 260, 500), Score: 0.9,
	}}, now)[0]

	if tracker.NeedFeature("camera", obs.TrackID, 0.30, now) {
		t.Fatal("low-quality face should not trigger SFace")
	}
	if !tracker.NeedFeature("camera", obs.TrackID, 0.62, now) {
		t.Fatal("first usable face should trigger SFace")
	}
	student := store.Student{ID: 7, StudentNo: "S007", Name: "甲"}
	tracker.RecordFeature("camera", obs.TrackID, 0.62, &student, 0.91, now)

	if tracker.NeedFeature("camera", obs.TrackID, 0.60, now.Add(150*time.Millisecond)) {
		t.Fatal("worse face should reuse cached identity")
	}
	if !tracker.NeedFeature("camera", obs.TrackID, 0.72, now.Add(300*time.Millisecond)) {
		t.Fatal("materially better face should refresh best-frame identity")
	}
	got, similarity, best := tracker.Identity("camera", obs.TrackID)
	if got == nil || got.ID != student.ID || similarity != 0.91 || best != 0.62 {
		t.Fatalf("unexpected cached identity: student=%#v similarity=%v best=%v", got, similarity, best)
	}
}

func TestFaceQualityRewardsLargeSharpFrontalFace(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 320, 240))
	for y := 0; y < 240; y++ {
		for x := 0; x < 320; x++ {
			if (x/4+y/4)%2 == 0 {
				img.Set(x, y, color.RGBA{R: 220, G: 220, B: 220, A: 255})
			} else {
				img.Set(x, y, color.RGBA{R: 70, G: 70, B: 70, A: 255})
			}
		}
	}
	detected := face.Detection{
		Rectangle: image.Rect(90, 45, 230, 195),
		Landmarks: [5]image.Point{
			{125, 95}, {195, 95}, {160, 125}, {135, 155}, {185, 155},
		},
		Score: 0.95,
	}
	quality := scoreFaceQuality(img, detected)
	if quality.Score < minRecognitionFaceQuality {
		t.Fatalf("good face quality too low: %#v", quality)
	}
	if quality.Status == "等待清晰人脸" {
		t.Fatalf("unexpected quality status: %#v", quality)
	}
}


func TestUnknownFaceRetriesSoonWithoutBusyLoop(t *testing.T) {
	tracker := newPersonTracker()
	now := time.Now()
	obs := tracker.Observe("camera", []person.Detection{{
		Rectangle: image.Rect(100, 80, 260, 500), Score: 0.9,
	}}, now)[0]
	tracker.RecordFeature("camera", obs.TrackID, 0.62, nil, 0, now)
	if tracker.NeedFeature("camera", obs.TrackID, 0.62, now.Add(400*time.Millisecond)) {
		t.Fatal("unknown face retried too soon")
	}
	if !tracker.NeedFeature("camera", obs.TrackID, 0.62, now.Add(650*time.Millisecond)) {
		t.Fatal("unknown face should retry within about 600ms")
	}
}


func TestPersonTrackerPredictsBetweenReusedDetectionFrames(t *testing.T) {
	tracker := newPersonTracker()
	now := time.Now()
	firstRect := image.Rect(100, 80, 260, 500)
	movedRect := image.Rect(120, 80, 280, 500)
	first := tracker.Observe("moving", []person.Detection{{Rectangle: firstRect, Score: 0.9}}, now)[0]
	tracker.Observe("moving", []person.Detection{{Rectangle: movedRect, Score: 0.9}}, now.Add(200*time.Millisecond))
	predicted := tracker.Observe("moving", []person.Detection{{Rectangle: movedRect, Score: 0.9}}, now.Add(300*time.Millisecond))[0]
	if predicted.TrackID != first.TrackID || !predicted.Predicted || predicted.Rectangle.Min.X <= movedRect.Min.X {
		t.Fatalf("reused detector frame did not advance the same moving track: %#v", predicted)
	}
}

func TestPersonTrackerPreservesIDsWhenPeopleCross(t *testing.T) {
	tracker := newPersonTracker()
	now := time.Now()
	first := tracker.Observe("crossing", []person.Detection{
		{Rectangle: image.Rect(100, 80, 180, 360), Score: 0.9},
		{Rectangle: image.Rect(300, 80, 380, 360), Score: 0.9},
	}, now)
	second := tracker.Observe("crossing", []person.Detection{
		{Rectangle: image.Rect(160, 80, 240, 360), Score: 0.9},
		{Rectangle: image.Rect(240, 80, 320, 360), Score: 0.9},
	}, now.Add(200*time.Millisecond))
	third := tracker.Observe("crossing", []person.Detection{
		{Rectangle: image.Rect(220, 80, 300, 360), Score: 0.9},
		{Rectangle: image.Rect(180, 80, 260, 360), Score: 0.9},
	}, now.Add(400*time.Millisecond))
	if len(first) != 2 || len(second) != 2 || len(third) != 2 ||
		second[0].TrackID != first[0].TrackID || second[1].TrackID != first[1].TrackID ||
		third[0].TrackID != first[0].TrackID || third[1].TrackID != first[1].TrackID {
		t.Fatalf("track IDs changed while people crossed: first=%#v second=%#v third=%#v", first, second, third)
	}
}

func TestPersonTrackerBrieflyKeepsABoxWhenDetectionIsMissed(t *testing.T) {
	tracker := newPersonTracker()
	now := time.Now()
	first := tracker.Observe("missed", []person.Detection{{Rectangle: image.Rect(100, 80, 260, 500), Score: 0.9}}, now)[0]
	predicted := tracker.Observe("missed", nil, now.Add(200*time.Millisecond))
	if len(predicted) != 1 || predicted[0].TrackID != first.TrackID || !predicted[0].Predicted {
		t.Fatalf("brief missed detection lost its predicted box: %#v", predicted)
	}
	expired := tracker.Observe("missed", nil, now.Add(personTrackPredictionGrace+100*time.Millisecond))
	if len(expired) != 0 {
		t.Fatalf("stale predicted box remained visible: %#v", expired)
	}
}

func TestAssociateFaceToPersonRejectsOverlappingAmbiguousBodies(t *testing.T) {
	body := image.Rect(100, 50, 300, 600)
	tracks := []personTrackObservation{
		{TrackID: "P1", Rectangle: body},
		{TrackID: "P2", Rectangle: body},
	}
	if _, ok := associateFaceToPerson(image.Rect(180, 100, 220, 150), tracks); ok {
		t.Fatal("face must not inherit a body track when two bodies are geometrically indistinguishable")
	}
}
