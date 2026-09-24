package web

import (
	"strings"
	"testing"

	"github.com/hwyc888/FaceSign/internal/store"
)

func TestCameraEndpointConsistencyCheckDetectsDifferentDevices(t *testing.T) {
	camera := store.Camera{
		StreamURL:   "rtsp://192.168.1.22:554/Streaming/channels/101",
		SnapshotURL: "http://192.168.1.21/ISAPI/Streaming/channels/1/picture",
	}
	check := cameraEndpointConsistencyCheck(camera)
	if check.Status != "error" {
		t.Fatalf("expected mismatch error, got %#v", check)
	}
	if !strings.Contains(check.Message, "192.168.1.22") || !strings.Contains(check.Message, "192.168.1.21") {
		t.Fatalf("mismatch message missing hosts: %s", check.Message)
	}
}

func TestCameraEndpointConsistencyCheckAcceptsSameDevice(t *testing.T) {
	camera := store.Camera{
		StreamURL:   "rtsp://192.168.1.22:554/Streaming/channels/101",
		SnapshotURL: "http://192.168.1.22/ISAPI/Streaming/channels/1/picture",
	}
	check := cameraEndpointConsistencyCheck(camera)
	if check.Status != "ok" {
		t.Fatalf("expected same-device ok, got %#v", check)
	}
}

func TestCameraQSVProbeUsesSameRealtimePipelineSettings(t *testing.T) {
	camera := store.Camera{Width: 1280, Height: 720}
	source := resolvedRTSPSource{Codec: "hevc"}
	args := strings.Join(webRTCH264OutputArgs(camera, source, webRTCH264Encoder{Name: "h264_qsv"}), " ")
	for _, want := range []string{"-c:v h264_qsv", "-g 30", "-bf 0", "-fps_mode passthrough"} {
		if !strings.Contains(args, want) {
			t.Fatalf("QSV realtime args missing %q: %s", want, args)
		}
	}
}
