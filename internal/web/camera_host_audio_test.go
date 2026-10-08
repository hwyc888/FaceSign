package web

import "testing"

func TestParseDShowAudioDevices(t *testing.T) {
	output := "[dshow @ 000001] DirectShow video devices\n[dshow @ 000001] \"USB Camera\"\n[dshow @ 000001] Alternative name \"@device_pnp_video\"\n[dshow @ 000001] DirectShow audio devices\n[dshow @ 000001] \"Microphone (USB Audio Device)\"\n[dshow @ 000001] Alternative name \"@device_cm_audio\"\n[dshow @ 000001] \"Microphone (Realtek Audio)\"\n[dshow @ 000001] Alternative name \"@device_cm_realtek\""
	devices := parseDShowAudioDevices(output)
	if len(devices) != 2 { t.Fatalf("got %d devices: %#v", len(devices), devices) }
	if devices[0].Name != "Microphone (USB Audio Device)" || devices[0].ID != "@device_cm_audio" { t.Fatalf("unexpected first audio device: %#v", devices[0]) }
}

func TestHostAudioMatchScorePrefersMatchingUSBDevice(t *testing.T) {
	usb := hostAudioMatchScore("Logitech USB Camera", "Microphone (Logitech USB Audio)")
	realtek := hostAudioMatchScore("Logitech USB Camera", "Microphone (Realtek Audio)")
	if usb <= realtek { t.Fatalf("expected matching USB audio score %d > unrelated score %d", usb, realtek) }
}
