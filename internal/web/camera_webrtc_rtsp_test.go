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
