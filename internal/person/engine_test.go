package person

import (
	"image"
	"testing"
)

func TestDecodePersonOutputFindsPerson(t *testing.T) {
	data := make([]float32, outputRows*outputCols)
	// First stride-8 grid cell: decoded center (80, 120), size 64x160.
	gx, gy, stride := 10, 15, 8
	row := gy*(inputSize/stride) + gx
	base := row * outputCols
	data[base] = 0
	data[base+1] = 0
	data[base+2] = 2.0794415 // ln(8) -> 64 px at stride 8
	data[base+3] = 2.9957323 // ln(20) -> 160 px
	data[base+4] = 0.9
	data[base+5+personClass] = 0.9

	got := decodePersonOutput(data, 1, image.Rect(0, 0, 416, 416), 0.35)
	if len(got) != 1 {
		t.Fatalf("detections=%d want=1", len(got))
	}
	if got[0].Score < 0.80 {
		t.Fatalf("score=%f", got[0].Score)
	}
	if got[0].Rectangle.Dx() < 60 || got[0].Rectangle.Dy() < 150 {
		t.Fatalf("unexpected person rectangle: %v", got[0].Rectangle)
	}
}

func TestNonMaximumSuppressionKeepsHighestScore(t *testing.T) {
	in := []Detection{
		{Rectangle: image.Rect(10, 10, 110, 210), Score: 0.91},
		{Rectangle: image.Rect(14, 14, 114, 214), Score: 0.72},
		{Rectangle: image.Rect(220, 20, 300, 200), Score: 0.80},
	}
	got := nonMaximumSuppression(in, 0.45, 24)
	if len(got) != 2 {
		t.Fatalf("detections=%d want=2", len(got))
	}
	if got[0].Score != 0.91 {
		t.Fatalf("highest score was not kept first: %#v", got)
	}
}
