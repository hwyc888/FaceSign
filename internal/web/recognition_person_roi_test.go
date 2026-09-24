package web

import (
	"image"
	"testing"

	"github.com/hwyc888/FaceSign/internal/person"
)

func TestScalePersonDetections(t *testing.T) {
	got := scalePersonDetections(
		[]person.Detection{{Rectangle: image.Rect(100, 50, 300, 450), Score: 0.9}},
		image.Rect(0, 0, 960, 540),
		image.Rect(0, 0, 1280, 720),
	)
	if len(got) != 1 {
		t.Fatalf("scaled detections=%d", len(got))
	}
	want := image.Rect(133, 67, 400, 600)
	if got[0].Rectangle != want {
		t.Fatalf("scaled rectangle=%v want=%v", got[0].Rectangle, want)
	}
}

func TestPersonFaceProbeWaitsUntilPersonIsCloseEnough(t *testing.T) {
	far := []person.Detection{{Rectangle: image.Rect(10, 10, 50, 160), Score: 0.9}}
	if anyPersonReadyForFaceProbe(far) {
		t.Fatal("far person should wait before main-stream face probing")
	}
	near := []person.Detection{{Rectangle: image.Rect(10, 10, 220, 500), Score: 0.9}}
	if !anyPersonReadyForFaceProbe(near) {
		t.Fatal("near person should trigger main-stream face probing")
	}
}

func TestPersonWaitingStatus(t *testing.T) {
	if got := personWaitingStatus(image.Rect(0, 0, 60, 220)); got != "等待靠近" {
		t.Fatalf("far status=%q", got)
	}
	if got := personWaitingStatus(image.Rect(0, 0, 300, 600)); got != "等待露脸" {
		t.Fatalf("near status=%q", got)
	}
}

func TestPersonHeadROIStaysNearUpperBody(t *testing.T) {
	bounds := image.Rect(0, 0, 1280, 720)
	personRect := image.Rect(400, 100, 700, 700)
	roi := personHeadROI(personRect, bounds)
	if roi.Empty() {
		t.Fatal("head ROI is empty")
	}
	if roi.Min.Y > personRect.Min.Y || roi.Max.Y >= personRect.Max.Y {
		t.Fatalf("ROI not limited to upper body: %v person=%v", roi, personRect)
	}
	if roi.Min.X < bounds.Min.X || roi.Max.X > bounds.Max.X {
		t.Fatalf("ROI exceeds bounds: %v", roi)
	}
}
