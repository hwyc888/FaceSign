package web

import (
	"testing"
	"time"
)

func TestCameraRecognitionGateAllowsOnlyOneTaskPerCamera(t *testing.T) {
	s := &Server{
		cameraRecognitionBusy:   make(map[int64]bool),
		cameraRecognitionNextAt: make(map[int64]time.Time),
	}
	if !s.tryBeginCameraRecognition(7) {
		t.Fatal("first recognition should acquire the camera gate")
	}
	if s.tryBeginCameraRecognition(7) {
		t.Fatal("second recognition on the same camera must be skipped")
	}
	if !s.tryBeginCameraRecognition(8) {
		t.Fatal("a different camera should have an independent recognition gate")
	}
	s.endCameraRecognition(7)
	if s.tryBeginCameraRecognition(7) {
		t.Fatal("camera gate must reserve a short idle window for realtime preview")
	}
	time.Sleep(cameraRecognitionMinIdle + 20*time.Millisecond)
	if !s.tryBeginCameraRecognition(7) {
		t.Fatal("camera gate did not reopen after the preview recovery window")
	}
}
