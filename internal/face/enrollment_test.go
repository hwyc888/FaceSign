package face

import (
	"errors"
	"image"
	"testing"

	onnxface "github.com/leandroveronezi/go-onnxface"
)

func TestSelectEnrollmentFaceIgnoresSmallBackgroundFace(t *testing.T) {
	primary := onnxface.Face{Rectangle: image.Rect(100, 80, 420, 400), Score: 0.95}
	background := onnxface.Face{Rectangle: image.Rect(10, 10, 100, 100), Score: 0.91}

	got, err := selectEnrollmentFace([]onnxface.Face{background, primary})
	if err != nil {
		t.Fatal(err)
	}
	if got.Rectangle != primary.Rectangle {
		t.Fatalf("selected %v, want primary %v", got.Rectangle, primary.Rectangle)
	}
}

func TestSelectEnrollmentFaceRejectsTwoSignificantFaces(t *testing.T) {
	first := onnxface.Face{Rectangle: image.Rect(100, 80, 420, 400), Score: 0.95}
	second := onnxface.Face{Rectangle: image.Rect(450, 100, 700, 350), Score: 0.92}

	_, err := selectEnrollmentFace([]onnxface.Face{first, second})
	if !errors.Is(err, ErrMultipleFaces) {
		t.Fatalf("got %v, want ErrMultipleFaces", err)
	}
}
