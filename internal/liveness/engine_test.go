package liveness

import (
	"os"
	"image"
	"image/color"
	"math"
	"testing"

	ort "github.com/yalue/onnxruntime_go"
)

func TestMiniFASModelSelectionAndRGBInput(t *testing.T) {
	if modelKindForPath("C:/models/minifasnet-v2se.onnx") != modelMiniFASV2SE || modelKindForPath("anti-spoof-mn3.onnx") != modelMN3 {
		t.Fatal("incorrect liveness model selection")
	}
	if got := (&Engine{kind:modelMiniFASV2SE}).ModelName(); got != "MiniFASNetV2-SE" { t.Fatalf("name: %q",got) }
	img := image.NewRGBA(image.Rect(0,0,inputSize,inputSize))
	img.SetRGBA(0,0,color.RGBA{R:255,G:128,B:64,A:255})
	data := make([]float32,3*inputSize*inputSize)
	fillMiniFASInput(data,img)
	const plane=inputSize*inputSize
	if data[0]!=1 || math.Abs(float64(data[plane]-float32(128.0/255)))>1e-6 || math.Abs(float64(data[2*plane]-float32(64.0/255)))>1e-6 {
		t.Fatalf("unexpected RGB/255 input: %v,%v,%v",data[0],data[plane],data[2*plane])
	}
	for _, a := range []struct{p,w int}{{-1,1},{-2,2},{12,10},{13,9}} {
		if got := reflect101(a.p,12); got != a.w {t.Fatalf("reflect101(%d)=%d",a.p,got)}
	}
	if got:=resizeMiniFASFaceCrop(img,image.Rect(0,0,12,12));got.Bounds()!=image.Rect(0,0,inputSize,inputSize) {
		t.Fatalf("crop size: %v",got.Bounds())
	}
}

// Exercised against the real downloaded ONNX and DLL by Windows CI.
func TestMiniFASModelRuntime(t *testing.T) {
	model, dll := os.Getenv("FACESIGN_TEST_MINIFAS_MODEL"), os.Getenv("FACESIGN_TEST_ONNX_DLL")
	if model == "" || dll == "" { t.Skip("requires actual packaged ONNX and runtime") }
	ort.SetSharedLibraryPath(dll)
	if err:=ort.InitializeEnvironment();err!=nil {t.Fatal(err)}
	defer ort.DestroyEnvironment()
	e,err:=New(model)
	if err!=nil {t.Fatalf("load MiniFAS ONNX: %v",err)}
	defer e.Close()
	img:=image.NewRGBA(image.Rect(0,0,256,256))
	for y:=0;y<256;y++ {for x:=0;x<256;x++ {img.SetRGBA(x,y,color.RGBA{R:uint8(x),G:uint8(y),B:127,A:255})}}
	score,err:=e.Score(img,image.Rect(80,80,155,169))
	if err!=nil {t.Fatalf("execute MiniFAS ONNX: %v",err)}
	if math.IsNaN(score)||score<0||score>1 {t.Fatalf("invalid score: %v",score)}
}

func TestExpandedFaceRectMatchesUpstreamCropRatio(t *testing.T) {
	bounds := image.Rect(0, 0, 640, 480)
	got := expandedFaceRect(image.Rect(100, 100, 200, 220), bounds)
	want := image.Rect(83, 79, 218, 241)
	if got != want {
		t.Fatalf("face crop=%v, want FaceSign MN3-compatible 1.35x1.35 crop %v", got, want)
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

func TestResizeFaceCropUsesRequestedRegionAndModelDimensions(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: 20, G: 40, B: 60, A: 255})
		}
	}
	crop := image.Rect(2, 2, 6, 6)
	got := resizeFaceCrop(img, crop)
	if got.Bounds() != image.Rect(0, 0, inputSize, inputSize) {
		t.Fatalf("resized crop bounds=%v", got.Bounds())
	}
	pixel := got.RGBAAt(inputSize/2, inputSize/2)
	if pixel != (color.RGBA{R: 20, G: 40, B: 60, A: 255}) {
		t.Fatalf("resized crop center=%v, want original crop color", pixel)
	}
}
