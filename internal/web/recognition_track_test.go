package web

import (
	"testing"
	"time"

	"github.com/hwyc888/FaceSign/internal/store"
)

func TestRecognitionTrackerRequiresMultipleLiveFrames(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 1, StudentNo: "S1", Name: "甲"}
	var decision trackDecision
	for i := 0; i < livenessMinFrames; i++ {
		decisions := tracker.Observe("camera-a", []recognitionObservation{{
			Student: student, Similarity: 0.91, LiveScore: 0.90,
		}}, now.Add(time.Duration(i)*300*time.Millisecond))
		decision = decisions[0]
		if i < livenessMinFrames-1 && decision.Verified {
			t.Fatalf("verified too early at frame %d", i+1)
		}
	}
	if !decision.Verified || !decision.NeedsAttendance || decision.LivenessStatus != "活体通过" {
		t.Fatalf("unexpected final decision: %#v", decision)
	}
	tracker.Commit("camera-a", decision.TrackID)
	decision = tracker.Observe("camera-a", []recognitionObservation{{
		Student: student, Similarity: 0.92, LiveScore: 0.88,
	}}, now.Add(2*time.Second))[0]
	if !decision.Verified || decision.NeedsAttendance {
		t.Fatalf("committed track should stay verified without another attendance write: %#v", decision)
	}
}

func TestRecognitionTrackerRejectsLowLiveness(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 2, StudentNo: "S2", Name: "乙"}
	var decision trackDecision
	for i := 0; i < 3; i++ {
		decision = tracker.Observe("camera-b", []recognitionObservation{{
			Student: student, Similarity: 0.93, LiveScore: 0.10,
		}}, now.Add(time.Duration(i)*250*time.Millisecond))[0]
	}
	if !decision.Rejected || decision.Verified || decision.LivenessStatus != "疑似照片/屏幕" {
		t.Fatalf("spoof track was not rejected: %#v", decision)
	}
}

func TestRecognitionTrackerExpiresOldTrack(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 3}
	first := tracker.Observe("camera-c", []recognitionObservation{{
		Student: student, Similarity: 0.9, LiveScore: 0.9,
	}}, now)[0]
	second := tracker.Observe("camera-c", []recognitionObservation{{
		Student: student, Similarity: 0.9, LiveScore: 0.9,
	}}, now.Add(livenessTrackTTL+time.Second))[0]
	if first.TrackID == second.TrackID || second.Frames != 1 {
		t.Fatalf("expired track was reused: first=%#v second=%#v", first, second)
	}
}
