package web

import "testing"

func TestParseDShowAudioDevicesCurrentFFmpegFormat(t *testing.T) {
	output := "[dshow @ 000001] \"Integrated Camera\" (video)\n" +
		"[dshow @ 000001]   Alternative name \"@device_pnp_video\"\n" +
		"[dshow @ 000001] \"Microphone (USB Audio Device)\" (audio)\n" +
		"[dshow @ 000001]   Alternative name \"@device_cm_audio\"\n" +
		"[dshow @ 000001] \"Microphone (Realtek Audio)\" (audio)\n" +
		"[dshow @ 000001]   Alternative name \"@device_cm_realtek\"\n"

	devices := parseDShowAudioDevices(output)
	if len(devices) != 2 {
		t.Fatalf("got %d devices: %#v", len(devices), devices)
	}
	if devices[0].Name != "Microphone (USB Audio Device)" || devices[0].ID != "@device_cm_audio" {
		t.Fatalf("unexpected first audio device: %#v", devices[0])
	}
	if devices[1].Name != "Microphone (Realtek Audio)" || devices[1].ID != "@device_cm_realtek" {
		t.Fatalf("unexpected second audio device: %#v", devices[1])
	}
}

func TestParseDShowAudioDevicesSectionFormat(t *testing.T) {
	output := "[dshow @ 000001] DirectShow video devices\n" +
		"[dshow @ 000001] \"USB Camera\"\n" +
		"[dshow @ 000001] Alternative name \"@device_pnp_video\"\n" +
		"[dshow @ 000001] DirectShow audio devices\n" +
		"[dshow @ 000001] \"Microphone (USB Audio Device)\"\n" +
		"[dshow @ 000001] Alternative name \"@device_cm_audio\"\n"

	devices := parseDShowAudioDevices(output)
	if len(devices) != 1 {
		t.Fatalf("got %d devices: %#v", len(devices), devices)
	}
	if devices[0].Name != "Microphone (USB Audio Device)" || devices[0].ID != "@device_cm_audio" {
		t.Fatalf("unexpected audio device: %#v", devices[0])
	}
}

func TestHostAudioMatchScorePrefersMatchingUSBDevice(t *testing.T) {
	usb := hostAudioMatchScore("Logitech USB Camera", "Microphone (Logitech USB Audio)")
	realtek := hostAudioMatchScore("Logitech USB Camera", "Microphone (Realtek Audio)")
	if usb <= realtek {
		t.Fatalf("expected matching USB audio score %d > unrelated score %d", usb, realtek)
	}
}


func TestHostAudioMatchScoreRejectsUnrelatedSystemMicrophone(t *testing.T) {
	if score := hostAudioMatchScore("Logitech C270", "Microphone (Realtek Audio)"); score != 0 {
		t.Fatalf("unrelated system microphone score=%d want 0", score)
	}
	if score := hostAudioMatchScore("USB2.0 Camera", "Microphone (USB Audio Device)"); score <= 0 {
		t.Fatalf("generic USB camera microphone should still match, score=%d", score)
	}
}


func TestParseWindowsPnPDeviceContainers(t *testing.T) {
	output := "USB2.0 Camera\tUSB\\VID_1234&PID_5678&MI_00\\1\tCamera\t{same-container}\r\n" +
		"Microphone (USB Audio Device)\tSWD\\MMDEVAPI\\MIC1\tAudioEndpoint\t{same-container}\r\n" +
		"Microphone (Realtek Audio)\tSWD\\MMDEVAPI\\MIC2\tAudioEndpoint\t{other-container}\r\n"
	devices := parseWindowsPnPDeviceContainers(output)
	if len(devices) != 3 {
		t.Fatalf("got %d PnP devices: %#v", len(devices), devices)
	}
	if devices[0].Name != "USB2.0 Camera" || devices[0].ContainerID != "{same-container}" {
		t.Fatalf("unexpected camera PnP record: %#v", devices[0])
	}
}

func TestMatchHostAudioDeviceByPnPContainerPrefersCameraMicrophone(t *testing.T) {
	audioDevices := []hostCameraDevice{
		{ID: "@device_cm_usb", Name: "Microphone (USB Audio Device)"},
		{ID: "@device_cm_realtek", Name: "Microphone (Realtek Audio)"},
	}
	pnpDevices := []windowsPnPDeviceContainer{
		{Name: "USB2.0 Camera", InstanceID: "USB\\VID_1234&PID_5678&MI_00\\1", Class: "Camera", ContainerID: "{same-container}"},
		{Name: "Microphone (USB Audio Device)", InstanceID: "SWD\\MMDEVAPI\\MIC1", Class: "AudioEndpoint", ContainerID: "{same-container}"},
		{Name: "Microphone (Realtek Audio)", InstanceID: "SWD\\MMDEVAPI\\MIC2", Class: "AudioEndpoint", ContainerID: "{other-container}"},
	}
	matched, ok := matchHostAudioDeviceByPnPContainer("USB2.0 Camera", audioDevices, pnpDevices)
	if !ok {
		t.Fatal("expected same-container USB microphone match")
	}
	if matched.ID != "@device_cm_usb" {
		t.Fatalf("matched wrong microphone: %#v", matched)
	}
}

func TestMatchHostAudioDeviceByPnPContainerDoesNotUseOtherPCMicrophone(t *testing.T) {
	audioDevices := []hostCameraDevice{{ID: "@device_cm_realtek", Name: "Microphone (Realtek Audio)"}}
	pnpDevices := []windowsPnPDeviceContainer{
		{Name: "USB Camera", InstanceID: "USB\\VID_1234&PID_5678&MI_00\\1", Class: "Camera", ContainerID: "{camera-container}"},
		{Name: "Microphone (Realtek Audio)", InstanceID: "SWD\\MMDEVAPI\\MIC2", Class: "AudioEndpoint", ContainerID: "{pc-container}"},
	}
	if matched, ok := matchHostAudioDeviceByPnPContainer("USB Camera", audioDevices, pnpDevices); ok {
		t.Fatalf("must not pair unrelated microphone: %#v", matched)
	}
}
