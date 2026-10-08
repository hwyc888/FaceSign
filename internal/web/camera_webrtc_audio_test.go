package web

import ("strings"; "testing")

func TestCameraWebRTCAudioCodecIsOpus(t *testing.T) {
	codec := cameraWebRTCAudioCodecCapability()
	if codec.MimeType != "audio/opus" || codec.ClockRate != 48000 || codec.Channels != 2 { t.Fatalf("unexpected audio codec: %#v", codec) }
}

func TestCameraWebRTCAudioFFmpegArgsUseOpusAndRTP(t *testing.T) {
	args := cameraWebRTCAudioFFmpegArgs([]string{"-rtsp_transport", "tcp", "-i", "rtsp://example.invalid/live"}, "rtp://127.0.0.1:5004?pkt_size=1200")
	joined := strings.Join(args, " ")
	for _, want := range []string{"-map 0:a:0", "-c:a libopus", "-ar 48000", "-ac 2", "-payload_type 111", "rtp://127.0.0.1:5004?pkt_size=1200"} {
		if !strings.Contains(joined, want) { t.Fatalf("audio ffmpeg args missing %q: %s", want, joined) }
	}
}
