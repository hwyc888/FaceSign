package web

import (
	"bytes"
	"testing"
)

func TestPillar2RecoveryChangesOnlyVideoCodecType(t *testing.T) {
	original := []byte("<?xml version=\"1.0\"?><StreamingChannel><Video><videoCodecType opt=\"H.264,H.265\">H.264</videoCodecType><maxFrameRate>2500</maxFrameRate><GovLength>25</GovLength><videoResolutionWidth>1280</videoResolutionWidth><videoResolutionHeight>720</videoResolutionHeight><constantBitRate>2048</constantBitRate></Video></StreamingChannel>")
	updated, ok := replaceHikvisionPillar2XMLValue(original, "videoCodecType", "H.265")
	if !ok { t.Fatal("videoCodecType was not replaced") }
	want := bytes.Replace(original, []byte(">H.264</videoCodecType>"), []byte(">H.265</videoCodecType>"), 1)
	if !bytes.Equal(updated, want) { t.Fatalf("recovery changed fields other than codec\nwant: %s\n got: %s", want, updated) }
}

func TestPillar2RecoveryCodecRecognition(t *testing.T) {
	for _, value := range []string{"H.265","H265","HEVC","hevc"} { if !isH265Codec(value) { t.Fatalf("expected H265 for %q", value) } }
	if isH265Codec("H.264") { t.Fatal("H.264 must not be treated as H.265") }
}
