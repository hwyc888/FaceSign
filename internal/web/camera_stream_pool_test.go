package web

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hwyc888/FaceSign/internal/store"
)

func TestMJPEGContinuousStreamFeedsSharedPool(t *testing.T) {
	var connections atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connections.Add(1)
		w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=frame")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("test server does not support flushing")
			return
		}
		for i := 0; ; i++ {
			img := image.NewRGBA(image.Rect(0, 0, 64, 48))
			img.Set(0, 0, color.RGBA{R: uint8(i % 255), G: 120, B: 40, A: 255})
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

	s := &Server{logger: slog.Default()}
	defer s.stopAllNetworkCameraStreams()
	camera := store.Camera{
		ID: 42, Name: "MJPEG shared", Kind: "network", Protocol: "mjpeg",
		StreamURL: upstream.URL, AuthMode: "none",
		Width: 1280, Height: 720, FPS: 6, TimeoutMS: 1000,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	frame, width, height, source, err := s.networkCameraFrame(ctx, camera)
	if err != nil {
		t.Fatal(err)
	}
	if len(frame) == 0 || width != 64 || height != 48 || source != "mjpeg" {
		t.Fatalf("unexpected first pooled frame: source=%q size=%dx%d bytes=%d", source, width, height, len(frame))
	}

	stream := s.ensureNetworkCameraStream(camera)
	first, ok := stream.current(networkCameraStreamFreshFor)
	if !ok {
		t.Fatal("continuous stream did not publish a frame")
	}
	time.Sleep(140 * time.Millisecond)
	second, ok := stream.current(networkCameraStreamFreshFor)
	if !ok || second.sequence <= first.sequence {
		t.Fatalf("shared pool did not keep advancing: first=%d second=%d", first.sequence, second.sequence)
	}

	if _, _, _, _, err := s.cameraPreviewSourceFrame(ctx, camera); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := s.cameraSourceFrame(ctx, camera); err != nil {
		t.Fatal(err)
	}
	if got := connections.Load(); got != 1 {
		t.Fatalf("preview and recognition must share one upstream continuous connection, got %d", got)
	}
}

func TestRTSPFailureFallsBackToSnapshot(t *testing.T) {
	t.Setenv("FACESIGN_FFMPEG", filepath.Join(t.TempDir(), "missing-ffmpeg"))

	snapshot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		img := image.NewRGBA(image.Rect(0, 0, 40, 30))
		_ = jpeg.Encode(w, img, nil)
	}))
	defer snapshot.Close()

	s := &Server{logger: slog.Default()}
	defer s.stopAllNetworkCameraStreams()
	camera := store.Camera{
		ID: 77, Name: "RTSP fallback", Kind: "network", Protocol: "rtsp",
		StreamURL: "rtsp://127.0.0.1:65530/live",
		SnapshotURL: snapshot.URL,
		AuthMode: "none", Width: 1280, Height: 720, FPS: 5, TimeoutMS: 500,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	frame, width, height, source, err := s.networkCameraFrame(ctx, camera)
	if err != nil {
		t.Fatal(err)
	}
	if len(frame) == 0 || width != 40 || height != 30 || source != "snapshot-fallback" {
		t.Fatalf("snapshot fallback failed: source=%q size=%dx%d bytes=%d", source, width, height, len(frame))
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		stream := s.ensureNetworkCameraStream(camera)
		if stream.error() != nil {
			if !strings.Contains(stream.error().Error(), "FFmpeg") {
				t.Fatalf("unexpected RTSP primary error: %v", stream.error())
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("RTSP stream error was not recorded")
}

func TestFFmpegRTSPInputAddsCredentialsWithoutChangingStoredURL(t *testing.T) {
	camera := store.Camera{
		StreamURL: "rtsp://192.168.1.64:554/live.sdp?profile=main",
		Username: "admin",
		Password: "p@ss:word",
		TimeoutMS: 3000,
	}
	input, err := ffmpegRTSPInputURL(camera)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.User == nil || parsed.User.Username() != "admin" {
		t.Fatalf("username missing from ffmpeg input: %s", input)
	}
	password, ok := parsed.User.Password()
	if !ok || password != "p@ss:word" {
		t.Fatal("password was not safely encoded in ffmpeg input URL")
	}
	if camera.StreamURL != "rtsp://192.168.1.64:554/live.sdp?profile=main" {
		t.Fatal("stored RTSP URL must not be mutated")
	}

	args := strings.Join(ffmpegRTSPArgs(camera, input), " ")
	for _, want := range []string{"-rtsp_transport tcp", "-rw_timeout", "-c:v mjpeg", "-f image2pipe", "pipe:1"} {
		if !strings.Contains(args, want) {
			t.Fatalf("ffmpeg RTSP args missing %q: %s", want, args)
		}
	}
	if strings.Contains(args, "fps=") {
		t.Fatal("RTSP decoder should preserve source frame cadence; preview sampling is handled by FaceSign")
	}
}

func TestReadJPEGSequencePublishesEveryFrame(t *testing.T) {
	var source bytes.Buffer
	for i := 0; i < 3; i++ {
		source.WriteString("--frame\r\nContent-Type: image/jpeg\r\n\r\n")
		img := image.NewRGBA(image.Rect(0, 0, 12+i, 8+i))
		if err := jpeg.Encode(&source, img, nil); err != nil {
			t.Fatal(err)
		}
		source.WriteString("\r\n")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	count := 0
	err := readJPEGSequence(ctx, &source, func(frame []byte) error {
		count++
		if _, _, err := image.Decode(bytes.NewReader(frame)); err != nil {
			return err
		}
		return nil
	})
	if !errorsIsEOF(err) {
		t.Fatalf("sequence read error=%v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 frames, got %d", count)
	}
}

func errorsIsEOF(err error) bool {
	return err == io.EOF
}
