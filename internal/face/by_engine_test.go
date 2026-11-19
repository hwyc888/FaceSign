package face

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func TestDecodeSCRFDDataFindsFaceAndFiveLandmarks(t *testing.T) {
	outputs := make([][]float32, 9)
	counts := []int{80 * 80 * 2, 40 * 40 * 2, 20 * 20 * 2}
	for i, count := range counts {
		outputs[i] = make([]float32, count)
		outputs[i+3] = make([]float32, count*4)
		outputs[i+6] = make([]float32, count*10)
	}
	outputs[0][0] = 0.99
	copy(outputs[3][:4], []float32{1, 1, 10, 10})
	copy(outputs[6][:10], []float32{5, 5, 10, 5, 7, 8, 5, 10, 10, 10})
	faces := decodeSCRFDData(outputs, 1, image.Rect(0, 0, 100, 100), 0.5)
	if len(faces) != 1 {
		t.Fatalf("expected one face, got %d", len(faces))
	}
	if faces[0].Rectangle.Empty() || faces[0].Score < 0.98 {
		t.Fatalf("unexpected face detection: %+v", faces[0])
	}
	if faces[0].Landmarks[0] != image.Pt(40, 40) || faces[0].Landmarks[4] != image.Pt(80, 80) {
		t.Fatalf("unexpected five-point landmarks: %+v", faces[0].Landmarks)
	}
}

func TestAlignArcFace112ReturnsNCHWFloatInput(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 112, 112))
	for y := 0; y < 112; y++ {
		for x := 0; x < 112; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 2), G: uint8(y * 2), B: 90, A: 255})
		}
	}
	landmarks := [5]image.Point{
		image.Pt(38, 52), image.Pt(74, 52), image.Pt(56, 72),
		image.Pt(42, 92), image.Pt(71, 92),
	}
	data, err := alignArcFace112(img, landmarks)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 3*112*112 {
		t.Fatalf("unexpected input length %d", len(data))
	}
	for _, value := range data {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			t.Fatalf("non-finite model input value %v", value)
		}
	}
}

func TestNormalizeFeatureL2(t *testing.T) {
	feature := []float32{3, 4}
	normalizeFeature(feature)
	if math.Abs(float64(feature[0])-0.6) > 1e-6 || math.Abs(float64(feature[1])-0.8) > 1e-6 {
		t.Fatalf("feature was not L2-normalized: %v", feature)
	}
}

func TestSimilaritySupportsArcFace512DimensionalVectors(t *testing.T) {
	a := make([]float32, 512)
	b := make([]float32, 512)
	a[0], b[0] = 1, 1
	if score := Similarity(a, b); math.Abs(score-1) > 1e-6 {
		t.Fatalf("identical vectors should score 1, got %f", score)
	}
	b[0] = -1
	if score := Similarity(a, b); score != 0 {
		t.Fatalf("opposite vectors should score 0 after score mapping, got %f", score)
	}
}

func TestSelectEnrollmentFaceDetectionChoosesDominantFace(t *testing.T) {
	faces := []Detection{
		{Rectangle: image.Rect(10, 10, 80, 80)},
		{Rectangle: image.Rect(85, 10, 100, 25)},
	}
	selected, err := selectEnrollmentFaceDetection(faces, true)
	if err != nil {
		t.Fatal(err)
	}
	if selected.Rectangle != faces[0].Rectangle {
		t.Fatalf("selected wrong face: %+v", selected.Rectangle)
	}
	if _, err := selectEnrollmentFaceDetection(faces, false); err != ErrMultipleFaces {
		t.Fatalf("expected multiple face error, got %v", err)
	}
}
