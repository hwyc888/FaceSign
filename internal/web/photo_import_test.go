package web

import (
	"archive/zip"
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
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


type photoImportTestUpload struct {
	field string
	name  string
	data  []byte
}

func newPhotoImportMultipartRequest(t *testing.T, uploads ...photoImportTestUpload) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, upload := range uploads {
		part, err := writer.CreateFormFile(upload.field, upload.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(upload.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/photo-import/analyze", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if err := req.ParseMultipartForm(32 << 20); err != nil {
		t.Fatal(err)
	}
	if req.MultipartForm != nil {
		t.Cleanup(func() { _ = req.MultipartForm.RemoveAll() })
	}
	return req
}

func TestPhotoImportSourcesAcceptMultipleDirectImages(t *testing.T) {
	first := []byte("first-image")
	second := []byte("second-image")
	req := newPhotoImportMultipartRequest(t,
		photoImportTestUpload{field: "files", name: "2026001_张三.jpg", data: first},
		photoImportTestUpload{field: "files", name: "2026002_李四.png", data: second},
	)
	sources, uploadName, closeSources, err := photoImportSourcesFromMultipart(req)
	if err != nil {
		t.Fatal(err)
	}
	defer closeSources()
	if len(sources) != 2 {
		t.Fatalf("got %d direct image sources, want 2", len(sources))
	}
	if uploadName != "直接上传 2 张图片" {
		t.Fatalf("unexpected upload label %q", uploadName)
	}
	for i, want := range [][]byte{first, second} {
		reader, err := sources[i].Open()
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("source %d content mismatch: got %q want %q", i, got, want)
		}
	}
	if sources[0].Name != "2026001_张三.jpg" || sources[1].Name != "2026002_李四.png" {
		t.Fatalf("direct image filenames were not preserved: %#v", sources)
	}
}

func TestPhotoImportSourcesKeepZIPSupport(t *testing.T) {
	imageBytes := []byte("zip-image-content")
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	entry, err := writer.Create("高一1班/2026001_张三.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(imageBytes); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := newPhotoImportMultipartRequest(t,
		photoImportTestUpload{field: "file", name: "students.zip", data: archive.Bytes()},
	)
	sources, uploadName, closeSources, err := photoImportSourcesFromMultipart(req)
	if err != nil {
		t.Fatal(err)
	}
	defer closeSources()
	if len(sources) != 1 || uploadName != "students.zip" {
		t.Fatalf("unexpected ZIP sources: count=%d name=%q", len(sources), uploadName)
	}
	if sources[0].Name != "高一1班/2026001_张三.jpg" {
		t.Fatalf("ZIP entry path was not preserved: %q", sources[0].Name)
	}
	reader, err := sources[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, imageBytes) {
		t.Fatalf("ZIP entry content mismatch: got %q want %q", got, imageBytes)
	}
}

func TestPhotoImportRejectsMixedZIPAndDirectFiles(t *testing.T) {
	req := newPhotoImportMultipartRequest(t,
		photoImportTestUpload{field: "file", name: "students.zip", data: []byte("zip")},
		photoImportTestUpload{field: "files", name: "2026001_张三.jpg", data: []byte("image")},
	)
	if _, _, _, err := photoImportSourcesFromMultipart(req); err == nil {
		t.Fatal("mixed ZIP and direct image uploads should be rejected")
	}
}
