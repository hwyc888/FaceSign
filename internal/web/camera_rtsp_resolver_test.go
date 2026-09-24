package web

import (
	"context"
	"net"
	"net/url"
	"strings"
	"testing"

	"github.com/hwyc888/FaceSign/internal/store"
)

func TestHikvisionRTSPCandidatesIncludeMainSubAndAlternatePort(t *testing.T) {
	camera := store.Camera{
		StreamURL: "rtsp://192.168.19.176:554/Streaming/channels/101",
		Username:  "admin",
		Password:  "p@ss:word",
	}
	candidates, err := rtspCandidates(camera)
	if err != nil {
		t.Fatal(err)
	}
	var sawMain, sawSub, sawISAPI, sawAlternatePort bool
	for _, candidate := range candidates {
		parsed, err := url.Parse(candidate.URL)
		if err != nil {
			t.Fatal(err)
		}
		if parsed.User == nil || parsed.User.Username() != "admin" {
			t.Fatalf("candidate lost username: %s", candidate.URL)
		}
		password, ok := parsed.User.Password()
		if !ok || password != "p@ss:word" {
			t.Fatalf("candidate lost password encoding: %s", candidate.URL)
		}
		switch strings.ToLower(parsed.Path) {
		case "/streaming/channels/101":
			sawMain = true
		case "/streaming/channels/102":
			sawSub = true
		case "/isapi/streaming/channels/101":
			sawISAPI = true
		}
		if parsed.Port() == "10554" {
			sawAlternatePort = true
		}
	}
	if !sawMain || !sawSub || !sawISAPI || !sawAlternatePort {
		t.Fatalf("missing Hikvision recovery candidates: main=%v sub=%v isapi=%v altPort=%v", sawMain, sawSub, sawISAPI, sawAlternatePort)
	}
}

func TestGenericRTSPCandidatesDoNotInventVendorPaths(t *testing.T) {
	camera := store.Camera{StreamURL: "rtsp://10.0.0.20:8554/live.sdp"}
	candidates, err := rtspCandidates(camera)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("generic RTSP should keep only configured URL, got %d", len(candidates))
	}
	if candidates[0].URL != camera.StreamURL {
		t.Fatalf("generic candidate changed: %s", candidates[0].URL)
	}
}

func TestRTSPDiagnosticRedactsPasswordAndClassifiesAuthFailure(t *testing.T) {
	camera := store.Camera{Username: "admin", Password: "Secret@123"}
	input := "rtsp://admin:Secret%40123@192.168.1.64:554/Streaming/channels/101"
	detail := classifyRTSPProbeFailure(
		"[rtsp] method DESCRIBE failed: 401 Unauthorized\n"+input,
		assertProbeError("exit status 1"),
		camera,
		input,
	)
	if !strings.Contains(detail, "认证失败") {
		t.Fatalf("expected authentication diagnosis, got %q", detail)
	}
	if strings.Contains(detail, camera.Password) {
		t.Fatalf("diagnostic leaked camera password: %q", detail)
	}
}

func TestInspectRTSPDigestAlgorithmDetectsSHA256(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 2048)
		_, _ = conn.Read(buf)
		_, _ = conn.Write([]byte("RTSP/1.0 401 Unauthorized\r\nCSeq: 1\r\nWWW-Authenticate: Digest realm=\"Hikvision\", nonce=\"abc\", algorithm=SHA-256, qop=\"auth\"\r\n\r\n"))
	}()

	algorithm := inspectRTSPDigestAlgorithm(context.Background(), "rtsp://admin:secret@"+listener.Addr().String()+"/Streaming/channels/101")
	if algorithm != "SHA-256" {
		t.Fatalf("digest algorithm=%q want SHA-256", algorithm)
	}
	<-done
}

func TestDetectFFmpegVideoCodec(t *testing.T) {
	tests := map[string]string{
		"Stream #0:0: Video: h264 (High)": "h264",
		"Stream #0:0: Video: hevc (Main)": "hevc",
		"Stream #0:0: Video: mjpeg":       "mjpeg",
		"no video":                         "",
	}
	for input, want := range tests {
		if got := detectFFmpegVideoCodec(input); got != want {
			t.Fatalf("detectFFmpegVideoCodec(%q)=%q want=%q", input, got, want)
		}
	}
}

type assertProbeError string

func (e assertProbeError) Error() string { return string(e) }


func TestHikvisionPurposePrefersMainForPreviewAndSubForRecognition(t *testing.T) {
	camera := store.Camera{
		StreamURL: "rtsp://192.168.19.176:554/Streaming/channels/101",
		Username: "admin",
		Password: "secret",
	}
	preview, err := rtspCandidatesForPurpose(camera, "preview")
	if err != nil {
		t.Fatal(err)
	}
	recognition, err := rtspCandidatesForPurpose(camera, "recognition")
	if err != nil {
		t.Fatal(err)
	}
	previewURL, err := url.Parse(preview[0].URL)
	if err != nil {
		t.Fatal(err)
	}
	recognitionURL, err := url.Parse(recognition[0].URL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(strings.ToLower(previewURL.Path), "/101") {
		t.Fatalf("preview must prefer Hikvision main stream 101, got %s", previewURL.Path)
	}
	if !strings.HasSuffix(strings.ToLower(recognitionURL.Path), "/102") {
		t.Fatalf("recognition must prefer Hikvision sub stream 102, got %s", recognitionURL.Path)
	}
}
