package web

import (
	"strings"
	"testing"

	"github.com/hwyc888/FaceSign/internal/store"
)

func TestParseDShowVideoDevices(t *testing.T) {
	output := `[dshow @ 000001] DirectShow video devices
[dshow @ 000001] "Integrated Camera" (video)
[dshow @ 000001]   Alternative name "@device_pnp_\\?\usb#vid_1234"
[dshow @ 000001] "USB Camera" (video)
[dshow @ 000001]   Alternative name "@device_pnp_\\?\usb#vid_abcd"
[dshow @ 000001] DirectShow audio devices
[dshow @ 000001] "Microphone" (audio)`
	devices := parseDShowVideoDevices(output)
	if len(devices) != 2 {
		t.Fatalf("got %d devices: %#v", len(devices), devices)
	}
	if devices[0].Name != "Integrated Camera" || !strings.Contains(devices[0].ID, "vid_1234") {
		t.Fatalf("unexpected first device: %#v", devices[0])
	}
	if devices[1].Name != "USB Camera" || !strings.Contains(devices[1].ID, "vid_abcd") {
		t.Fatalf("unexpected second device: %#v", devices[1])
	}
}

func TestParseDShowVideoDevicesWithoutVideoSuffix(t *testing.T) {
	output := `[dshow @ 000001] DirectShow video devices (some may be both video and audio devices)
[dshow @ 000001]  "Integrated Camera"
[dshow @ 000001]     Alternative name "@device_pnp_\\?\usb#vid_13d3"
[dshow @ 000001]  "OBS Virtual Camera"
[dshow @ 000001]     Alternative name "@device_sw_{860BB310}"
[dshow @ 000001] DirectShow audio devices
[dshow @ 000001]  "Microphone"`
	devices := parseDShowVideoDevices(output)
	if len(devices) != 2 {
		t.Fatalf("got %d devices: %#v", len(devices), devices)
	}
	if devices[0].Name != "Integrated Camera" || !strings.Contains(devices[0].ID, "vid_13d3") {
		t.Fatalf("unexpected first device: %#v", devices[0])
	}
	if devices[1].Name != "OBS Virtual Camera" || !strings.Contains(devices[1].ID, "860BB310") {
		t.Fatalf("unexpected second device: %#v", devices[1])
	}
}

func TestParseDShowVideoDevicesMixedOutputFormats(t *testing.T) {
	output := `[dshow @ 000001] DirectShow video devices
[dshow @ 000001] "Legacy USB Camera" (video)
[dshow @ 000001] Alternative name "@device_pnp_legacy"
[dshow @ 000001] "Modern USB Camera"
[dshow @ 000001] Alternative name "@device_pnp_modern"
[dshow @ 000001] DirectShow audio devices
[dshow @ 000001] "USB Microphone" (audio)`
	devices := parseDShowVideoDevices(output)
	if len(devices) != 2 {
		t.Fatalf("got %d devices: %#v", len(devices), devices)
	}
	if devices[0].Name != "Legacy USB Camera" || devices[1].Name != "Modern USB Camera" {
		t.Fatalf("unexpected devices: %#v", devices)
	}
}

func TestHostCameraFFmpegArgsUseDirectShowAndSelectedDevice(t *testing.T) {
	args := hostCameraFFmpegArgs(store.Camera{Width: 1280, Height: 720, FPS: 30}, "@device_pnp_test")
	joined := strings.Join(args, " ")
	for _, want := range []string{"-f dshow", "video=@device_pnp_test", "fps=30", "scale=1280:720", "-f image2pipe"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("host camera args missing %q: %s", want, joined)
		}
	}
}
