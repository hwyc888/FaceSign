package web

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hwyc888/FaceSign/internal/store"
)

func TestNetworkCameraPreviewRateIsIndependentFromRecognition(t *testing.T) {
	camera := store.Camera{Kind: "network", FPS: 5}
	if got, want := networkCameraFrameInterval(camera), 200*time.Millisecond; got != want {
		t.Fatalf("recognition interval=%v want=%v", got, want)
	}
	if got, want := cameraPreviewFrameInterval(camera), 40*time.Millisecond; got != want {
		t.Fatalf("preview interval=%v want=%v", got, want)
	}
}

func TestNetworkCameraJPEGPassThrough(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 18, 12))
	img.Set(1, 1, color.RGBA{B: 255, A: 255})
	var original bytes.Buffer
	if err := jpeg.Encode(&original, img, &jpeg.Options{Quality: 87}); err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(original.Bytes())
	}))
	defer upstream.Close()

	camera := store.Camera{
		Kind: "network", Protocol: "http_snapshot", SnapshotURL: upstream.URL,
		AuthMode: "none", TimeoutMS: 2000,
	}
	frame, width, height, err := fetchNetworkCameraFrame(context.Background(), camera)
	if err != nil {
		t.Fatal(err)
	}
	if width != 18 || height != 12 {
		t.Fatalf("frame size=%dx%d", width, height)
	}
	if !bytes.Equal(frame, original.Bytes()) {
		t.Fatal("JPEG frame was unnecessarily decoded and re-encoded")
	}
}

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


func TestNetworkCameraLiveStream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		img := image.NewRGBA(image.Rect(0, 0, 36, 22))
		img.Set(3, 3, color.RGBA{G: 255, A: 255})
		if err := jpeg.Encode(w, img, nil); err != nil {
			t.Error(err)
		}
	}))
	defer upstream.Close()

	st, err := store.Open(filepath.Join(t.TempDir(), "camera-stream.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	camera, err := st.CreateCamera(context.Background(), store.CameraInput{
		Name: "Live camera", Kind: "network", Protocol: "http_snapshot",
		SnapshotURL: upstream.URL, AuthMode: "none",
		Width: 1280, Height: 720, FPS: 8, TimeoutMS: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}

	s := &Server{store: st, logger: slog.Default()}
	server := httptest.NewServer(http.HandlerFunc(s.cameraAction))
	defer server.Close()

	resp, err := server.Client().Get(fmt.Sprintf("%s/api/cameras/%d/stream", server.URL, camera.ID))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream status=%d", resp.StatusCode)
	}
	mediaType, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	if mediaType != "multipart/x-mixed-replace" || params["boundary"] == "" {
		t.Fatalf("unexpected stream content type: %s", resp.Header.Get("Content-Type"))
	}

	reader := multipart.NewReader(resp.Body, params["boundary"])
	part, err := reader.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	decoded, _, err := image.Decode(part)
	if err != nil {
		t.Fatalf("decode stream frame: %v", err)
	}
	if decoded.Bounds().Dx() != 36 || decoded.Bounds().Dy() != 22 {
		t.Fatalf("unexpected live stream frame size: %v", decoded.Bounds())
	}
}


func TestNetworkCameraContinuousPreviewAdvancesWithoutDuplicatePollingFrames(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=frame")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("test server does not support flushing")
			return
		}
		for i := 0; ; i++ {
			img := image.NewRGBA(image.Rect(0, 0, 40+(i%20), 24))
			var frame bytes.Buffer
			if err := jpeg.Encode(&frame, img, &jpeg.Options{Quality: 80}); err != nil {
				t.Error(err)
				return
			}
			if _, err := fmt.Fprintf(w, "--frame\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", frame.Len()); err != nil {
				return
			}
			if _, err := w.Write(frame.Bytes()); err != nil {
				return
			}
			if _, err := w.Write([]byte("\r\n")); err != nil {
				return
			}
			flusher.Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(35 * time.Millisecond):
			}
		}
	}))
	defer upstream.Close()

	st, err := store.Open(filepath.Join(t.TempDir(), "camera-continuous-preview.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	camera, err := st.CreateCamera(context.Background(), store.CameraInput{
		Name: "Continuous preview", Kind: "network", Protocol: "mjpeg",
		StreamURL: upstream.URL, AuthMode: "none",
		Width: 1280, Height: 720, FPS: 5, TimeoutMS: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}

	s := &Server{store: st, logger: slog.Default()}
	defer s.stopAllNetworkCameraStreams()
	server := httptest.NewServer(http.HandlerFunc(s.cameraAction))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/cameras/%d/stream", server.URL, camera.ID), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	_, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	reader := multipart.NewReader(resp.Body, params["boundary"])
	widths := make([]int, 0, 2)
	for len(widths) < 2 {
		part, err := reader.NextPart()
		if err != nil {
			t.Fatal(err)
		}
		img, _, err := image.Decode(part)
		_ = part.Close()
		if err != nil {
			t.Fatal(err)
		}
		widths = append(widths, img.Bounds().Dx())
	}
	if widths[0] == widths[1] {
		t.Fatalf("continuous preview repeated the same pooled frame instead of waiting for a new one: widths=%v", widths)
	}
}

func TestNetworkCameraPreviewReusesConfiguredFrameCache(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		w.Header().Set("Content-Type", "image/jpeg")
		img := image.NewRGBA(image.Rect(0, 0, 28, 18))
		_ = jpeg.Encode(w, img, nil)
	}))
	defer upstream.Close()

	camera := store.Camera{
		ID: 42, Name: "Stable preview", Kind: "network", Protocol: "http_snapshot",
		SnapshotURL: upstream.URL, AuthMode: "none",
		Width: 1280, Height: 720, FPS: 8, TimeoutMS: 2000,
	}
	s := &Server{logger: slog.Default(), networkCameraFrames: make(map[int64]*networkCameraFrameCache)}
	for i := 0; i < 2; i++ {
		frame, _, _, status, err := s.cameraPreviewSourceFrame(context.Background(), camera)
		if err != nil || status != http.StatusOK || len(frame) == 0 {
			t.Fatalf("preview frame %d failed: status=%d bytes=%d err=%v", i+1, status, len(frame), err)
		}
	}
	if upstreamCalls != 1 {
		t.Fatalf("preview should reuse the configured frame cache instead of hammering snapshot URL; calls=%d", upstreamCalls)
	}
}

func TestNetworkCameraLiveStreamSurvivesTransientSnapshotFailure(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := upstreamCalls.Add(1)
		if call == 2 {
			http.Error(w, "temporary camera overload", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		img := image.NewRGBA(image.Rect(0, 0, 34, 20))
		img.Set(2, 2, color.RGBA{R: 255, G: 128, A: 255})
		_ = jpeg.Encode(w, img, nil)
	}))
	defer upstream.Close()

	st, err := store.Open(filepath.Join(t.TempDir(), "camera-stream-retry.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	camera, err := st.CreateCamera(context.Background(), store.CameraInput{
		Name: "Retry camera", Kind: "network", Protocol: "http_snapshot",
		SnapshotURL: upstream.URL, AuthMode: "none",
		Width: 1280, Height: 720, FPS: 12, TimeoutMS: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}

	s := &Server{store: st, logger: slog.Default(), networkCameraFrames: make(map[int64]*networkCameraFrameCache)}
	server := httptest.NewServer(http.HandlerFunc(s.cameraAction))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/cameras/%d/stream", server.URL, camera.ID), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	_, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	reader := multipart.NewReader(resp.Body, params["boundary"])
	for i := 0; i < 8; i++ {
		part, err := reader.NextPart()
		if err != nil {
			t.Fatalf("stream ended after transient upstream failure at part %d: %v", i+1, err)
		}
		if _, _, err := image.Decode(part); err != nil {
			t.Fatalf("decode recovered stream frame %d: %v", i+1, err)
		}
		_ = part.Close()
	}
	if upstreamCalls.Load() < 3 {
		t.Fatalf("preview did not retry after the temporary snapshot failure; upstream calls=%d", upstreamCalls.Load())
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


func TestCameraAgentUploadAndFrame(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "camera-agent-web.db"))
	if err != nil { t.Fatal(err) }
	defer st.Close()
	secret := "agent-test-secret-0123456789"
	camera, err := st.CreateCamera(context.Background(), store.CameraInput{
		Name: "Agent camera", Kind: "agent", AgentID: "classroom-test",
		AgentSecretHash: hashCameraAgentSecret(secret), Width: 1280, Height: 720, FPS: 2,
	})
	if err != nil { t.Fatal(err) }
	s := &Server{store: st, logger: slog.Default(), cameraAgentFrames: make(map[string]cameraAgentFrame)}
	img := image.NewRGBA(image.Rect(0, 0, 20, 12))
	var body bytes.Buffer
	if err := jpeg.Encode(&body, img, nil); err != nil { t.Fatal(err) }
	upload := httptest.NewRequest(http.MethodPost, "/api/camera-agents/frame", bytes.NewReader(body.Bytes()))
	upload.Header.Set("X-FaceSign-Agent-ID", "classroom-test")
	upload.Header.Set("Authorization", "Bearer "+secret)
	uploadRec := httptest.NewRecorder()
	s.cameraAgentFrameUpload(uploadRec, upload)
	if uploadRec.Code != http.StatusOK { t.Fatalf("upload status=%d body=%s", uploadRec.Code, uploadRec.Body.String()) }
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/cameras/%d/frame", camera.ID), nil)
	s.cameraAction(rec, req)
	if rec.Code != http.StatusOK { t.Fatalf("agent frame status=%d body=%s", rec.Code, rec.Body.String()) }
	if rec.Header().Get("Content-Type") != "image/jpeg" { t.Fatalf("unexpected content type: %s", rec.Header().Get("Content-Type")) }
}

func TestCameraAgentRejectsWrongSecret(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "camera-agent-auth.db"))
	if err != nil { t.Fatal(err) }
	defer st.Close()
	_, err = st.CreateCamera(context.Background(), store.CameraInput{
		Name: "Agent auth", Kind: "agent", AgentID: "classroom-auth",
		AgentSecretHash: hashCameraAgentSecret("correct-secret"), Width: 1280, Height: 720, FPS: 2,
	})
	if err != nil { t.Fatal(err) }
	s := &Server{store: st, logger: slog.Default(), cameraAgentFrames: make(map[string]cameraAgentFrame)}
	req := httptest.NewRequest(http.MethodPost, "/api/camera-agents/frame", strings.NewReader("bad"))
	req.Header.Set("X-FaceSign-Agent-ID", "classroom-auth")
	req.Header.Set("Authorization", "Bearer wrong-secret")
	rec := httptest.NewRecorder()
	s.cameraAgentFrameUpload(rec, req)
	if rec.Code != http.StatusUnauthorized { t.Fatalf("expected unauthorized, got %d", rec.Code) }
}


func TestCameraConnectionTestDiagnosesSuccess(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		img := image.NewRGBA(image.Rect(0, 0, 40, 30))
		if err := jpeg.Encode(w, img, nil); err != nil {
			t.Error(err)
		}
	}))
	defer upstream.Close()

	st, err := store.Open(filepath.Join(t.TempDir(), "camera-test-success.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := &Server{store: st, logger: slog.Default()}

	body := fmt.Sprintf(`{"name":"test","kind":"network","protocol":"http_snapshot","snapshot_url":%q,"auth_mode":"none","width":1280,"height":720,"fps":5,"timeout_ms":2000}`, upstream.URL)
	req := httptest.NewRequest(http.MethodPost, "/api/cameras/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.cameraTest(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"ok":true`) ||
		!strings.Contains(rec.Body.String(), `"width":40`) ||
		!strings.Contains(rec.Body.String(), `"height":30`) ||
		!strings.Contains(rec.Body.String(), `"preview_base64"`) {
		t.Fatalf("unexpected success diagnostics: %s", rec.Body.String())
	}
}

func TestCameraConnectionTestDiagnosesAuthenticationFailure(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer upstream.Close()

	st, err := store.Open(filepath.Join(t.TempDir(), "camera-test-auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := &Server{store: st, logger: slog.Default()}

	body := fmt.Sprintf(`{"name":"test","kind":"network","protocol":"http_snapshot","snapshot_url":%q,"username":"admin","password":"wrong","auth_mode":"basic","width":1280,"height":720,"fps":5,"timeout_ms":2000}`, upstream.URL)
	req := httptest.NewRequest(http.MethodPost, "/api/cameras/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.cameraTest(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"ok":false`) ||
		!strings.Contains(rec.Body.String(), "认证失败") ||
		!strings.Contains(rec.Body.String(), `"name":"网络连接","status":"ok"`) ||
		!strings.Contains(rec.Body.String(), `"name":"身份认证","status":"error"`) {
		t.Fatalf("authentication failure was not diagnosed: %s", rec.Body.String())
	}
}

func TestCameraConnectionTestDiagnosesInvalidImage(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>camera login page</html>"))
	}))
	defer upstream.Close()

	st, err := store.Open(filepath.Join(t.TempDir(), "camera-test-image.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := &Server{store: st, logger: slog.Default()}

	body := fmt.Sprintf(`{"name":"test","kind":"network","protocol":"http_snapshot","snapshot_url":%q,"auth_mode":"none","width":1280,"height":720,"fps":5,"timeout_ms":2000}`, upstream.URL)
	req := httptest.NewRequest(http.MethodPost, "/api/cameras/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.cameraTest(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "没有返回有效图片") ||
		!strings.Contains(rec.Body.String(), `"name":"图像抓取","status":"error"`) {
		t.Fatalf("invalid image was not diagnosed: %s", rec.Body.String())
	}
}


func TestNetworkCameraAutoDetectsDigest(t *testing.T) {
	authorized := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Digest ") {
			w.Header().Set("WWW-Authenticate", `Digest realm="camera", nonce="auto-digest", qop="auth", algorithm=MD5`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		authorized = true
		w.Header().Set("Content-Type", "image/jpeg")
		img := image.NewRGBA(image.Rect(0, 0, 18, 12))
		_ = jpeg.Encode(w, img, nil)
	}))
	defer upstream.Close()

	camera := store.Camera{
		Name: "Auto Digest", Kind: "network", Protocol: "http_snapshot",
		SnapshotURL: upstream.URL, Username: "admin", Password: "pass",
		AuthMode: "auto", Width: 1280, Height: 720, FPS: 5, TimeoutMS: 2000,
	}
	frame, width, height, detectedAuth, err := fetchNetworkCameraFrameWithAuth(context.Background(), camera)
	if err != nil {
		t.Fatal(err)
	}
	if !authorized || detectedAuth != "digest" || len(frame) == 0 || width != 18 || height != 12 {
		t.Fatalf("auto digest failed: authorized=%v auth=%q size=%dx%d bytes=%d", authorized, detectedAuth, width, height, len(frame))
	}
}

func TestNetworkCameraAutoDetectsBasic(t *testing.T) {
	authorized := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "admin" || password != "pass" {
			w.Header().Set("WWW-Authenticate", `Basic realm="camera"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		authorized = true
		w.Header().Set("Content-Type", "image/jpeg")
		img := image.NewRGBA(image.Rect(0, 0, 20, 14))
		_ = jpeg.Encode(w, img, nil)
	}))
	defer upstream.Close()

	camera := store.Camera{
		Name: "Auto Basic", Kind: "network", Protocol: "http_snapshot",
		SnapshotURL: upstream.URL, Username: "admin", Password: "pass",
		AuthMode: "auto", Width: 1280, Height: 720, FPS: 5, TimeoutMS: 2000,
	}
	frame, width, height, detectedAuth, err := fetchNetworkCameraFrameWithAuth(context.Background(), camera)
	if err != nil {
		t.Fatal(err)
	}
	if !authorized || detectedAuth != "basic" || len(frame) == 0 || width != 20 || height != 14 {
		t.Fatalf("auto basic failed: authorized=%v auth=%q size=%dx%d bytes=%d", authorized, detectedAuth, width, height, len(frame))
	}
}

func TestNetworkCameraAutoDetectsNoAuthentication(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		img := image.NewRGBA(image.Rect(0, 0, 16, 10))
		_ = jpeg.Encode(w, img, nil)
	}))
	defer upstream.Close()

	camera := store.Camera{
		Name: "Auto None", Kind: "network", Protocol: "http_snapshot",
		SnapshotURL: upstream.URL, AuthMode: "auto",
		Width: 1280, Height: 720, FPS: 5, TimeoutMS: 2000,
	}
	_, _, _, detectedAuth, err := fetchNetworkCameraFrameWithAuth(context.Background(), camera)
	if err != nil {
		t.Fatal(err)
	}
	if detectedAuth != "none" {
		t.Fatalf("expected no authentication, got %q", detectedAuth)
	}
}

func TestCameraConnectionTestReportsAutoDetectedDigest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Digest ") {
			w.Header().Set("WWW-Authenticate", `Digest realm="camera", nonce="test-report", qop="auth", algorithm=MD5`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		img := image.NewRGBA(image.Rect(0, 0, 22, 15))
		_ = jpeg.Encode(w, img, nil)
	}))
	defer upstream.Close()

	st, err := store.Open(filepath.Join(t.TempDir(), "camera-auto-test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := &Server{store: st, logger: slog.Default()}

	body := fmt.Sprintf(`{"name":"auto","kind":"network","protocol":"http_snapshot","snapshot_url":%q,"username":"admin","password":"pass","auth_mode":"auto","width":1280,"height":720,"fps":5,"timeout_ms":2000}`, upstream.URL)
	req := httptest.NewRequest(http.MethodPost, "/api/cameras/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.cameraTest(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	bodyText := rec.Body.String()
	if !strings.Contains(bodyText, `"ok":true`) ||
		!strings.Contains(bodyText, `"detected_auth":"digest"`) ||
		!strings.Contains(bodyText, "自动检测到 Digest") {
		t.Fatalf("auto auth result not reported: %s", bodyText)
	}
}


func TestNetworkCameraFrameCacheSharesUpstreamFetch(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		w.Header().Set("Content-Type", "image/jpeg")
		img := image.NewRGBA(image.Rect(0, 0, 24, 16))
		_ = jpeg.Encode(w, img, nil)
	}))
	defer upstream.Close()

	st, err := store.Open(filepath.Join(t.TempDir(), "camera-cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	camera, err := st.CreateCamera(context.Background(), store.CameraInput{
		Name: "Cached camera", Kind: "network", Protocol: "http_snapshot",
		SnapshotURL: upstream.URL, AuthMode: "none",
		Width: 1280, Height: 720, FPS: 5, TimeoutMS: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}

	s := &Server{store: st, logger: slog.Default()}
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/cameras/%d/frame", camera.ID), nil)
		s.cameraAction(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("frame %d status=%d body=%s", i+1, rec.Code, rec.Body.String())
		}
	}
	if upstreamCalls != 1 {
		t.Fatalf("preview and recognition should share a cached upstream frame; calls=%d", upstreamCalls)
	}

	time.Sleep(networkCameraFrameInterval(camera) + 30*time.Millisecond)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/cameras/%d/frame", camera.ID), nil)
	s.cameraAction(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("refreshed frame status=%d body=%s", rec.Code, rec.Body.String())
	}
	if upstreamCalls != 2 {
		t.Fatalf("cache should refresh after one frame interval; calls=%d", upstreamCalls)
	}
}

func TestRestoreH265RouteRejectsOtherCameraNames(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "restore-h265-guard.db"))
	if err != nil { t.Fatal(err) }
	defer st.Close()
	camera, err := st.CreateCamera(context.Background(), store.CameraInput{Name:"其他摄像头", Kind:"network", Protocol:"rtsp", StreamURL:"rtsp://127.0.0.1:554/Streaming/channels/101", SnapshotURL:"http://127.0.0.1/ISAPI/Streaming/channels/1/picture", AuthMode:"none", Width:1280, Height:720, FPS:5, TimeoutMS:1000})
	if err != nil { t.Fatal(err) }
	s := &Server{store: st}
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/cameras/%d/restore-h265", camera.ID), nil)
	rec := httptest.NewRecorder()
	s.cameraAction(rec, req)
	if rec.Code != http.StatusBadGateway { t.Fatalf("unexpected status=%d body=%s", rec.Code, rec.Body.String()) }
	if !strings.Contains(rec.Body.String(), "只允许恢复“大门主校道立柱2”") { t.Fatalf("unexpected response: %s", rec.Body.String()) }
}
