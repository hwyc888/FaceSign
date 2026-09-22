package liveness

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func TestExpandedFaceRect(t *testing.T) {
	bounds := image.Rect(0, 0, 640, 480)
	got := expandedFaceRect(image.Rect(100, 100, 200, 220), bounds)
	if got.Empty() || got.Min.X >= 100 || got.Min.Y >= 100 || got.Max.X <= 200 || got.Max.Y <= 220 {
		t.Fatalf("face crop was not expanded: %v", got)
	}
	if !got.In(bounds) {
		t.Fatalf("expanded face left image bounds: %v", got)
	}
}

func TestFillInputUsesRGBNormalization(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, inputSize, inputSize))
	for y := 0; y < inputSize; y++ {
		for x := 0; x < inputSize; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 128, B: 64, A: 255})
		}
	}
	data := make([]float32, 3*inputSize*inputSize)
	fillInput(data, img)
	wantR := float32((255.0 - 151.2405) / 63.0105)
	wantG := float32((128.0 - 119.5950) / 56.4570)
	wantB := float32((64.0 - 107.8395) / 55.0035)
	if math.Abs(float64(data[0]-wantR)) > 1e-4 ||
		math.Abs(float64(data[inputSize*inputSize]-wantG)) > 1e-4 ||
		math.Abs(float64(data[2*inputSize*inputSize]-wantB)) > 1e-4 {
		t.Fatalf("unexpected normalized values: %v %v %v", data[0], data[inputSize*inputSize], data[2*inputSize*inputSize])
	}
}

func TestRealProbability(t *testing.T) {
	if got := realProbability(0.9, 0.1); math.Abs(got-0.9) > 1e-6 {
		t.Fatalf("probability output changed: %v", got)
	}
	got := realProbability(2, 0)
	if got <= 0.8 || got >= 1 {
		t.Fatalf("softmax conversion unexpected: %v", got)
	}
}
