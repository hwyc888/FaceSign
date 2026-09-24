package web

import (
	"strings"
	"testing"

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


func TestWebRTCH264OutputArgsCompatibilityTranscodesH264(t *testing.T) {
	camera := store.Camera{Width: 1280, Height: 720}
	source := resolvedRTSPSource{Codec: "h264"}
	encoder := webRTCH264Encoder{
		Name:      "libopenh264",
		Mode:      "WebRTC H.264兼容软件转码",
		Transcode: true,
	}
	args := strings.Join(webRTCH264OutputArgs(camera, source, encoder), " ")

	for _, want := range []string{
		"-vf scale=1280:720:force_original_aspect_ratio=decrease:force_divisible_by=2,format=yuv420p",
		"-c:v libopenh264",
		"-bf 0",
	} {
		if !strings.Contains(args, want) {
			t.Fatalf("H264 compatibility transcode missing %q: %s", want, args)
		}
	}
	if strings.Contains(args, "-c:v copy") {
		t.Fatalf("forced H264 compatibility path must transcode: %s", args)
	}
}
