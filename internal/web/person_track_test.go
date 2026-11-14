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

func TestPersonTrackerRevalidatesKnownIdentityOnEveryUsableFrame(t *testing.T) {
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
	first := store.Student{ID: 7, StudentNo: "S007", Name: "甲"}
	tracker.RecordFeature("camera", obs.TrackID, 0.62, &first, 0.91, now)

	if !tracker.NeedFeature("camera", obs.TrackID, 0.60, now.Add(150*time.Millisecond)) {
		t.Fatal("known identity must be revalidated on the next usable frame")
	}
	second := store.Student{ID: 8, StudentNo: "S008", Name: "乙"}
	tracker.RecordFeature("camera", obs.TrackID, 0.60, &second, 0.75, now.Add(150*time.Millisecond))
	got, similarity, best := tracker.Identity("camera", obs.TrackID)
	if got == nil || got.ID != second.ID || similarity != 0.75 || best != 0.62 {
		t.Fatalf("fresh identity did not replace stale identity: student=%#v similarity=%v best=%v", got, similarity, best)
	}

	tracker.RecordFeature("camera", obs.TrackID, 0.60, nil, 0, now.Add(300*time.Millisecond))
	got, similarity, best = tracker.Identity("camera", obs.TrackID)
	if got != nil || similarity != 0 || best != 0.62 {
		t.Fatalf("unmatched fresh frame must clear the previous identity: student=%#v similarity=%v best=%v", got, similarity, best)
	}
}

func TestPersonTrackerDoesNotExposeCachedIdentityInNewObservation(t *testing.T) {
	tracker := newPersonTracker()
	now := time.Now()
	detection := person.Detection{Rectangle: image.Rect(100, 80, 260, 500), Score: 0.9}
	obs := tracker.Observe("camera", []person.Detection{detection}, now)[0]
	student := store.Student{ID: 7, StudentNo: "S007", Name: "甲"}
	tracker.RecordFeature("camera", obs.TrackID, 0.62, &student, 0.91, now)

	next := tracker.Observe("camera", []person.Detection{detection}, now.Add(100*time.Millisecond))[0]
	if next.Student != nil || next.Similarity != 0 {
		t.Fatalf("person observation must not reuse an identity without a fresh face feature: %#v", next)
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


func TestFaceQualityExplainsLowLightAndBlur(t *testing.T) {
	detected := face.Detection{
		Rectangle: image.Rect(90, 45, 230, 195),
		Landmarks: [5]image.Point{{125, 95}, {195, 95}, {160, 125}, {135, 155}, {185, 155}},
		Score: 0.95,
	}

	dark := image.NewRGBA(image.Rect(0, 0, 320, 240))
	for y := 0; y < 240; y++ {
		for x := 0; x < 320; x++ {
			dark.Set(x, y, color.RGBA{R: 18, G: 18, B: 18, A: 255})
		}
	}
	if got := scoreFaceQuality(dark, detected).Hint; got != "光线太暗，请增加正面补光" {
		t.Fatalf("low-light hint=%q", got)
	}

	blurred := image.NewRGBA(image.Rect(0, 0, 320, 240))
	for y := 0; y < 240; y++ {
		for x := 0; x < 320; x++ {
			blurred.Set(x, y, color.RGBA{R: 125, G: 125, B: 125, A: 255})
		}
	}
	if got := scoreFaceQuality(blurred, detected).Hint; got != "画面模糊，请擦拭镜头并保持静止" {
		t.Fatalf("blur hint=%q", got)
	}
}
