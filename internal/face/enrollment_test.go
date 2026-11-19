package face

import (
	"errors"
	"image"
	"testing"
)

func TestSelectEnrollmentFaceIgnoresSmallBackgroundFace(t *testing.T) {
	primary := Detection{Rectangle: image.Rect(100, 80, 420, 400), Score: 0.95}
	background := Detection{Rectangle: image.Rect(10, 10, 100, 100), Score: 0.91}
	got, err := selectEnrollmentFaceDetection([]Detection{background, primary}, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Rectangle != primary.Rectangle {
		t.Fatalf("selected %v, want primary %v", got.Rectangle, primary.Rectangle)
	}
}

func TestSelectEnrollmentFaceRejectsTwoSignificantFaces(t *testing.T) {
	first := Detection{Rectangle: image.Rect(100, 80, 420, 400), Score: 0.95}
	second := Detection{Rectangle: image.Rect(450, 100, 700, 350), Score: 0.92}
	_, err := selectEnrollmentFaceDetection([]Detection{first, second}, true)
	if !errors.Is(err, ErrMultipleFaces) {
		t.Fatalf("got %v, want ErrMultipleFaces", err)
	}
}
