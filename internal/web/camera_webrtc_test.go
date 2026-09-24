package web

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hwyc888/FaceSign/internal/store"
)

func TestWebRTCH264OutputArgsPassThroughH264(t *testing.T) {
	camera := store.Camera{Width: 1280, Height: 720}
	source := resolvedRTSPSource{Codec: "h264"}
	args := strings.Join(webRTCH264OutputArgs(camera, source, webRTCH264Encoder{}), " ")

	for _, want := range []string{
		"-c:v copy",
		"-bsf:v h264_mp4toannexb,dump_extra=freq=keyframe",
	} {
		if !strings.Contains(args, want) {
			t.Fatalf("H264 direct path missing %q: %s", want, args)
		}
	}
	if strings.Contains(args, "-vf ") {
		t.Fatalf("H264 direct path must not transcode: %s", args)
	}
}

func TestWebRTCH264OutputArgsTranscodeHEVC(t *testing.T) {
	camera := store.Camera{Width: 1280, Height: 720}
	source := resolvedRTSPSource{Codec: "hevc"}
	encoder := webRTCH264Encoder{Name: "libopenh264", Mode: "WebRTC H.265→H.264软件转码"}
	args := strings.Join(webRTCH264OutputArgs(camera, source, encoder), " ")

	for _, want := range []string{
		"-vf scale=1280:720:force_original_aspect_ratio=decrease:force_divisible_by=2,format=yuv420p",
		"-c:v libopenh264",
		"-b:v 2500k",
		"-maxrate 3000k",
		"-bufsize 1500k",
		"-g 30",
		"-bf 0",
		"-fps_mode passthrough",
	} {
		if !strings.Contains(args, want) {
			t.Fatalf("HEVC transcode path missing %q: %s", want, args)
		}
	}
	if strings.Contains(args, "-c:v copy") {
		t.Fatalf("HEVC path must transcode to H264: %s", args)
	}
}

func TestWebRTCH264OutputArgsUsesHigherBitrateFor1080p(t *testing.T) {
	camera := store.Camera{Width: 1920, Height: 1080}
	source := resolvedRTSPSource{Codec: "hevc"}
	encoder := webRTCH264Encoder{Name: "libopenh264"}
	args := strings.Join(webRTCH264OutputArgs(camera, source, encoder), " ")
	if !strings.Contains(args, "-b:v 4000k") || !strings.Contains(args, "-maxrate 4500k") {
		t.Fatalf("1080p transcode bitrate missing: %s", args)
	}
}


func TestWebRTCH264QSVZeroCopyInputAndOutputArgs(t *testing.T) {
	camera := store.Camera{Width: 1280, Height: 720, TimeoutMS: 3000}
	source := resolvedRTSPSource{Codec: "hevc", Transport: "tcp", URL: "rtsp://camera/Streaming/channels/101"}
	encoder := webRTCH264Encoder{Name: "h264_qsv", QSVZeroCopy: true}

	input := strings.Join(webRTCH264InputArgs(camera, source, encoder), " ")
	for _, want := range []string{
		"-hwaccel qsv",
		"-hwaccel_output_format qsv",
		"-c:v hevc_qsv",
		"-rtsp_transport tcp",
	} {
		if !strings.Contains(input, want) {
			t.Fatalf("QSV zero-copy input missing %q: %s", want, input)
		}
	}

	output := strings.Join(webRTCH264OutputArgs(camera, source, encoder), " ")
	for _, want := range []string{
		"-vf scale_qsv=w=1280:h=720:format=nv12",
		"-c:v h264_qsv",
		"-preset veryfast",
		"-look_ahead 0",
		"-async_depth 2",
		"-bf 0",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("QSV zero-copy output missing %q: %s", want, output)
		}
	}
	if strings.Contains(output, "scale=1280:720") || strings.Contains(output, "format=yuv420p") {
		t.Fatalf("QSV zero-copy path must not fall back through CPU scale/format: %s", output)
	}
}

func TestCameraPreviewHubSharesOneWorkerAcrossSubscribers(t *testing.T) {
	oldStreamFn := cameraPreviewPacketStreamFn
	oldIdle := cameraPreviewHubIdleTimeout
	oldPrepare := cameraPreviewHubPrepareTimeout
	cameraPreviewHubIdleTimeout = 25 * time.Millisecond
	cameraPreviewHubPrepareTimeout = time.Second
	var starts atomic.Int32
	cameraPreviewPacketStreamFn = func(ctx context.Context, camera store.Camera, source resolvedRTSPSource, encoder webRTCH264Encoder, writePacket func([]byte) error) error {
		starts.Add(1)
		<-ctx.Done()
		return ctx.Err()
	}
	t.Cleanup(func() {
		cameraPreviewPacketStreamFn = oldStreamFn
		cameraPreviewHubIdleTimeout = oldIdle
		cameraPreviewHubPrepareTimeout = oldPrepare
	})

	s := &Server{cameraPreviewHubs: make(map[string]*cameraPreviewHub)}
	camera := store.Camera{ID: 42, StreamURL: "rtsp://camera/Streaming/channels/101", Width: 1280, Height: 720}
	hub := s.getOrCreateCameraPreviewHub(camera, resolvedRTSPSource{Codec: "h264"}, webRTCH264Encoder{Mode: "copy"})

	track1, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000}, "video1", "hub")
	if err != nil {
		t.Fatal(err)
	}
	track2, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000}, "video2", "hub")
	if err != nil {
		t.Fatal(err)
	}

	unsubscribe1 := s.subscribeCameraPreviewHub(hub, track1)
	unsubscribe2 := s.subscribeCameraPreviewHub(hub, track2)

	deadline := time.Now().Add(time.Second)
	for starts.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := starts.Load(); got != 1 {
		t.Fatalf("two subscribers started %d preview workers, want 1", got)
	}

	unsubscribe1()
	unsubscribe2()

	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s.cameraPreviewHubMu.Lock()
		remaining := len(s.cameraPreviewHubs)
		s.cameraPreviewHubMu.Unlock()
		if remaining == 0 {
			if got := starts.Load(); got != 1 {
				t.Fatalf("idle cleanup restarted preview worker: starts=%d", got)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("shared preview hub was not cleaned up after the last subscriber left")
}

func TestCameraPreviewHubFallsBackWhenQSVZeroCopyFails(t *testing.T) {
	oldStreamFn := cameraPreviewPacketStreamFn
	var calls atomic.Int32
	cameraPreviewPacketStreamFn = func(ctx context.Context, camera store.Camera, source resolvedRTSPSource, encoder webRTCH264Encoder, writePacket func([]byte) error) error {
		call := calls.Add(1)
		if call == 1 {
			if !encoder.QSVZeroCopy {
				t.Fatal("first preview attempt must use QSV zero-copy")
			}
			return errors.New("qsv device failed")
		}
		if encoder.QSVZeroCopy {
			t.Fatal("fallback preview must disable QSV zero-copy")
		}
		<-ctx.Done()
		return ctx.Err()
	}
	t.Cleanup(func() { cameraPreviewPacketStreamFn = oldStreamFn })

	s := &Server{cameraPreviewHubs: make(map[string]*cameraPreviewHub)}
	camera := store.Camera{ID: 7, StreamURL: "rtsp://camera/101"}
	hub := s.getOrCreateCameraPreviewHub(camera, resolvedRTSPSource{Codec: "hevc"}, webRTCH264Encoder{Name: "h264_qsv", QSVZeroCopy: true})

	track, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000}, "video", "hub")
	if err != nil {
		t.Fatal(err)
	}
	unsubscribe := s.subscribeCameraPreviewHub(hub, track)
	defer unsubscribe()

	deadline := time.Now().Add(time.Second)
	for calls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if calls.Load() < 2 {
		t.Fatal("QSV zero-copy failure did not trigger fallback")
	}
	_, encoder := hub.configuration()
	if encoder.QSVZeroCopy {
		t.Fatal("hub still reports QSV zero-copy after fallback")
	}
}
