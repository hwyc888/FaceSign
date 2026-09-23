package web

import (
	"bytes"
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
