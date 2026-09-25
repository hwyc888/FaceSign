package web

import (
	"strings"
	"testing"

	"github.com/bluenviron/gortsplib/v5/pkg/format"
)

func TestWebRTCH264NativeCapabilityUsesCameraProfileAndPacketization(t *testing.T) {
	source := webRTCRTSPSource{
		resolvedRTSPSource: resolvedRTSPSource{Codec: "h264"},
		H264: &format.H264{
			PacketizationMode: 1,
			SPS:               []byte{0x67, 0x4d, 0x00, 0x28},
			PPS:               []byte{0x68, 0xee, 0x3c, 0x80},
		},
	}
	capability := webRTCH264CodecCapability(source)
	if capability.MimeType != "video/H264" || capability.ClockRate != 90000 {
		t.Fatalf("unexpected H264 capability: %#v", capability)
	}
	for _, want := range []string{
		"level-asymmetry-allowed=1",
		"packetization-mode=1",
		"profile-level-id=4d0028",
	} {
		if !strings.Contains(capability.SDPFmtpLine, want) {
			t.Fatalf("native H264 capability missing %q: %s", want, capability.SDPFmtpLine)
		}
	}
}

func TestWebRTCH264TranscodeCapabilityRemainsGeneric(t *testing.T) {
	capability := webRTCH264CodecCapability(webRTCRTSPSource{
		resolvedRTSPSource: resolvedRTSPSource{Codec: "hevc"},
	})
	if capability.SDPFmtpLine != "" {
		t.Fatalf("HEVC transcode capability must remain generic, got %q", capability.SDPFmtpLine)
	}
}

func TestWebRTCH265NativeCapability(t *testing.T) {
	capability := webRTCH265CodecCapability()
	if capability.MimeType != "video/H265" || capability.ClockRate != 90000 {
		t.Fatalf("unexpected H265 capability: %#v", capability)
	}
	if capability.SDPFmtpLine != "" {
		t.Fatalf("H265 direct capability should stay generic for browser negotiation, got %q", capability.SDPFmtpLine)
	}
}

func TestWebRTCOfferSupportsH265(t *testing.T) {
	tests := []struct {
		name string
		sdp  string
		want bool
	}{
		{name: "h265", sdp: "v=0\r\na=rtpmap:116 H265/90000\r\n", want: true},
		{name: "hevc alias", sdp: "v=0\na=rtpmap:116 HEVC/90000\n", want: true},
		{name: "h264 only", sdp: "v=0\r\na=rtpmap:102 H264/90000\r\n", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := webRTCOfferSupportsH265(tt.sdp); got != tt.want {
				t.Fatalf("webRTCOfferSupportsH265()=%v want %v", got, tt.want)
			}
		})
	}
}
