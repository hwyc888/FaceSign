package web

import (
	"image"
	"testing"

	"github.com/hwyc888/FaceSign/internal/face"
)

func TestPhotoImportIdentity(t *testing.T) {
	tests := []struct {
		file      string
		studentNo string
		name      string
	}{
		{"2026001_张三.jpg", "2026001", "张三"},
		{"A2026-李四.png", "A2026", "李四"},
		{"王五.webp", "", "王五"},
	}
	for _, tt := range tests {
		studentNo, name := photoImportIdentity(tt.file)
		if studentNo != tt.studentNo || name != tt.name {
			t.Fatalf("%s => (%q,%q), want (%q,%q)", tt.file, studentNo, name, tt.studentNo, tt.name)
		}
	}
}

func TestPhotoImportClassHint(t *testing.T) {
	classes := map[string]bool{"高一1班": true, "高一2班": true}
	if got := photoImportClassHint("高一2班/2026001_张三.jpg", classes); got != "高一2班" {
		t.Fatalf("unexpected class hint %q", got)
	}
	if got := photoImportClassHint("其他目录/张三.jpg", classes); got != "" {
		t.Fatalf("unexpected class hint %q", got)
	}
}

func TestAssessImportedPhotoRequiresGoodFrontalFace(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 800, 1000))
	good := face.Detection{
		Rectangle: image.Rect(270, 250, 530, 570),
		Landmarks: [5]image.Point{
			{X: 340, Y: 350},
			{X: 460, Y: 350},
			{X: 400, Y: 410},
			{X: 355, Y: 475},
			{X: 445, Y: 475},
		},
		Score: 0.98,
	}
	quality, _, ok := assessImportedPhoto(img, good)
	if !ok || quality == "不合格" {
		t.Fatalf("good frontal photo rejected: quality=%q", quality)
	}

	side := good
	side.Landmarks[2].X = 455
	if _, _, ok := assessImportedPhoto(img, side); ok {
		t.Fatal("strong side pose should not be accepted as a frontal import sample")
	}

	tiny := good
	tiny.Rectangle = image.Rect(380, 450, 430, 510)
	if _, _, ok := assessImportedPhoto(img, tiny); ok {
		t.Fatal("tiny face should not be accepted")
	}
}

func TestSupportedPhotoFormats(t *testing.T) {
	for _, format := range []string{"jpeg", "png", "webp", "gif", "bmp", "tiff"} {
		if !supportedPhotoFormat(format) {
			t.Fatalf("expected format %q to be supported", format)
		}
	}
}
