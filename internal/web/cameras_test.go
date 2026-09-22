package web

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hwyc888/FaceSign/internal/store"
)

func TestNetworkCameraFrameProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		img := image.NewRGBA(image.Rect(0, 0, 32, 24))
		img.Set(2, 2, color.RGBA{R: 255, A: 255})
		if err := jpeg.Encode(w, img, nil); err != nil {
			t.Error(err)
		}
	}))
	defer upstream.Close()

	st, err := store.Open(filepath.Join(t.TempDir(), "camera-web.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	camera, err := st.CreateCamera(context.Background(), store.CameraInput{
		Name: "HTTP camera",
		Kind: "network",
		Protocol: "http_snapshot",
		SnapshotURL: upstream.URL,
		Width: 1280,
		Height: 720,
		FPS: 5,
		TimeoutMS: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}

	s := &Server{store: st, logger: slog.Default()}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/cameras/%d/frame", camera.ID), nil)
	s.cameraAction(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("frame status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("unexpected content type: %s", rec.Header().Get("Content-Type"))
	}
	decoded, _, err := image.Decode(rec.Body)
	if err != nil {
		t.Fatalf("decode proxied frame: %v", err)
	}
	if decoded.Bounds().Dx() != 32 || decoded.Bounds().Dy() != 24 {
		t.Fatalf("unexpected frame size: %v", decoded.Bounds())
	}
}

func TestNetworkCameraDigestChallenge(t *testing.T) {
	authorized := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Digest ") {
			w.Header().Set("WWW-Authenticate", `Digest realm="camera", nonce="abcdef", qop="auth", algorithm=MD5`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		authorized = true
		w.Header().Set("Content-Type", "image/jpeg")
		img := image.NewRGBA(image.Rect(0, 0, 8, 8))
		_ = jpeg.Encode(w, img, nil)
	}))
	defer upstream.Close()

	st, err := store.Open(filepath.Join(t.TempDir(), "camera-digest.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	camera, err := st.CreateCamera(context.Background(), store.CameraInput{
		Name: "Digest camera",
		Kind: "network",
		Protocol: "http_snapshot",
		SnapshotURL: upstream.URL,
		Username: "admin",
		Password: "pass",
		AuthMode: "digest",
		Width: 1280,
		Height: 720,
		FPS: 5,
		TimeoutMS: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}

	frame, _, _, err := fetchNetworkCameraFrame(context.Background(), camera)
	if err != nil {
		t.Fatal(err)
	}
	if len(frame) == 0 || !authorized {
		t.Fatal("digest authenticated frame was not fetched")
	}
}
