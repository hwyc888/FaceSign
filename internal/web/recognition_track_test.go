package web

import (
	"testing"
	"time"

	"github.com/hwyc888/FaceSign/internal/store"
)

func TestRecognitionTrackerFastPassesThreeStrongLiveFrames(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 1, StudentNo: "S1", Name: "甲"}
	var decision trackDecision
	for i := 0; i < livenessFastFrames; i++ {
		decision = tracker.Observe("camera-fast", []recognitionObservation{{
			Student: student, Similarity: 0.91, LiveScore: 0.90, Exposure: 1, Sharpness: 1,
		}}, now.Add(time.Duration(i)*180*time.Millisecond))[0]
		if i < livenessFastFrames-1 && decision.Verified {
			t.Fatalf("verified too early at frame %d", i+1)
		}
	}
	if !decision.Verified || decision.Rejected || decision.TimedOut || !decision.NeedsAttendance {
		t.Fatalf("strong live track did not fast-pass: %#v", decision)
	}
	if decision.Frames != livenessFastFrames || decision.LivenessStatus != "活体通过" {
		t.Fatalf("unexpected fast-pass decision: %#v", decision)
	}

	tracker.Commit("camera-fast", decision.TrackID)
	decision = tracker.Observe("camera-fast", []recognitionObservation{{
		Student: student, Similarity: 0.92, LiveScore: 0.88, Exposure: 1, Sharpness: 1,
	}}, now.Add(1500*time.Millisecond))[0]
	if !decision.Verified || decision.NeedsAttendance {
		t.Fatalf("committed track should stay verified without another attendance write: %#v", decision)
	}
}

func TestRecognitionTrackerNormalPassesFourModerateLiveFrames(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 2, StudentNo: "S2", Name: "乙"}
	scores := []float64{0.70, 0.69, 0.67, 0.71}
	var decision trackDecision
	for i, score := range scores {
		decision = tracker.Observe("camera-normal", []recognitionObservation{{
			Student: student, Similarity: 0.92, LiveScore: score, Exposure: 1, Sharpness: 1,
		}}, now.Add(time.Duration(i)*220*time.Millisecond))[0]
		if i < len(scores)-1 && decision.Verified {
			t.Fatalf("moderate track verified before normal frame %d", i+1)
		}
	}
	if !decision.Verified || decision.Frames != livenessMinFrames || decision.TimedOut {
		t.Fatalf("moderate live track did not pass on frame %d: %#v", livenessMinFrames, decision)
	}
}

func TestRecognitionTrackerRejectsLowLivenessInThreeFrames(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 3, StudentNo: "S3", Name: "丙"}
	var decision trackDecision
	for i := 0; i < 3; i++ {
		decision = tracker.Observe("camera-spoof", []recognitionObservation{{
			Student: student, Similarity: 0.93, LiveScore: 0.10, Exposure: 1, Sharpness: 1,
		}}, now.Add(time.Duration(i)*180*time.Millisecond))[0]
	}
	if !decision.Rejected || decision.Verified || decision.TimedOut || decision.LivenessStatus != "疑似照片/屏幕" {
		t.Fatalf("spoof track was not rejected quickly: %#v", decision)
	}
}

func TestRecognitionTrackerTimesOutWithinTwoSecondsAndResets(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 4, StudentNo: "S4", Name: "丁"}
	var decision trackDecision
	for i, offset := range []time.Duration{0, 600 * time.Millisecond, 1200 * time.Millisecond, 2050 * time.Millisecond} {
		decision = tracker.Observe("camera-timeout", []recognitionObservation{{
			Student: student, Similarity: 0.91, LiveScore: 0.55, Exposure: 1, Sharpness: 1,
		}}, now.Add(offset))[0]
		if i < 3 && decision.TimedOut {
			t.Fatalf("track timed out too early at observation %d: %#v", i+1, decision)
		}
	}
	if !decision.TimedOut || decision.Verified || decision.Rejected || decision.LivenessStatus != "活体超时，请重新对准" {
		t.Fatalf("uncertain track did not time out cleanly: %#v", decision)
	}

	next := tracker.Observe("camera-timeout", []recognitionObservation{{
		Student: student, Similarity: 0.91, LiveScore: 0.90, Exposure: 1, Sharpness: 1,
	}}, now.Add(2200*time.Millisecond))[0]
	if next.TrackID == decision.TrackID || next.Frames != 1 {
		t.Fatalf("timed-out track was not reset: old=%#v new=%#v", decision, next)
	}
}

func TestRecognitionTrackerStopsUncertainTrackAtMaxFrames(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 5, StudentNo: "S5", Name: "戊"}
	var decision trackDecision
	for i := 0; i < livenessMaxFrames; i++ {
		decision = tracker.Observe("camera-max", []recognitionObservation{{
			Student: student, Similarity: 0.90, LiveScore: 0.62, Exposure: 1, Sharpness: 1,
		}}, now.Add(time.Duration(i)*150*time.Millisecond))[0]
	}
	if !decision.TimedOut || decision.Frames != livenessMaxFrames || decision.Verified || decision.Rejected {
		t.Fatalf("uncertain track did not stop at max frames: %#v", decision)
	}
}

func TestRecognitionTrackerExpiresOldTrack(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 6}
	first := tracker.Observe("camera-expire", []recognitionObservation{{
		Student: student, Similarity: 0.9, LiveScore: 0.9, Exposure: 1, Sharpness: 1,
	}}, now)[0]
	second := tracker.Observe("camera-expire", []recognitionObservation{{
		Student: student, Similarity: 0.9, LiveScore: 0.9, Exposure: 1, Sharpness: 1,
	}}, now.Add(livenessTrackTTL+time.Second))[0]
	if first.TrackID == second.TrackID || second.Frames != 1 {
		t.Fatalf("expired track was reused: first=%#v second=%#v", first, second)
	}
}


func TestRecognitionTrackerDoesNotTreatPoorQualityFramesAsSpoofEvidence(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 77, StudentNo: "S77", Name: "测试学生"}
	var decision trackDecision
	for i := 0; i < 4; i++ {
		decision = tracker.Observe("camera-low-light", []recognitionObservation{{
			Student: student, Similarity: 0.93, LiveScore: 0.05,
			Exposure: 0.12, Sharpness: 0.04,
		}}, now.Add(time.Duration(i)*180*time.Millisecond))[0]
		if decision.Rejected || decision.Verified || decision.Frames != 0 {
			t.Fatalf("poor quality frame must not count as spoof/live evidence: %#v", decision)
		}
		if decision.LivenessStatus != "光线不足或画面模糊，请调整摄像头" {
			t.Fatalf("unexpected low-quality status: %#v", decision)
		}
	}

	for i := 0; i < livenessFastFrames; i++ {
		decision = tracker.Observe("camera-low-light", []recognitionObservation{{
			Student: student, Similarity: 0.93, LiveScore: 0.90,
			Exposure: 0.9, Sharpness: 0.8,
		}}, now.Add(900*time.Millisecond+time.Duration(i)*180*time.Millisecond))[0]
	}
	if !decision.Verified || decision.Rejected {
		t.Fatalf("good quality live frames should still pass normally: %#v", decision)
	}
}


func TestRecognitionTrackerLowLightModeAcceptsDimFramesButStillRequiresLiveScores(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 88, StudentNo: "S88", Name: "夜间测试"}
	var decision trackDecision
	for i := 0; i < livenessMinFrames; i++ {
		decision = tracker.Observe("camera-low-light-enabled", []recognitionObservation{{
			Student: student, Similarity: 0.90, LiveScore: 0.70,
			Exposure: 0.25, Sharpness: 0.055, LowLightMode: true,
		}}, now.Add(time.Duration(i)*250*time.Millisecond))[0]
	}
	if !decision.Verified || decision.Rejected {
		t.Fatalf("usable dim live frames should pass in low-light mode: %#v", decision)
	}
	if decision.Frames != livenessMinFrames {
		t.Fatalf("low-light mode unexpectedly changed live vote count: %#v", decision)
	}
}

func TestRecognitionTrackerLowLightModeStillRejectsPhotoAndScreenScores(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 89, StudentNo: "S89", Name: "照片攻击测试"}
	var decision trackDecision
	for i := 0; i < livenessFastFrames; i++ {
		decision = tracker.Observe("camera-low-light-spoof", []recognitionObservation{{
			Student: student, Similarity: 0.94, LiveScore: 0.10,
			Exposure: 0.25, Sharpness: 0.15, LowLightMode: true,
		}}, now.Add(time.Duration(i)*200*time.Millisecond))[0]
	}
	if !decision.Rejected || decision.Verified || decision.NeedsAttendance {
		t.Fatalf("low-light mode must continue rejecting spoof evidence: %#v", decision)
	}
}

func TestRecognitionTrackerLowLightModeAutoPassesUncertainScores(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 90, StudentNo: "S90", Name: "夜间自动签到测试"}
	var decision trackDecision
	for i := 0; i < livenessMinFrames; i++ {
		decision = tracker.Observe("camera-low-light-uncertain", []recognitionObservation{{
			Student: student, Similarity: 0.90, LiveScore: 0.55,
			Exposure: 0.25, Sharpness: 0.055, LowLightMode: true,
		}}, now.Add(time.Duration(i)*250*time.Millisecond))[0]
		if i < livenessMinFrames-1 && decision.Verified {
			t.Fatalf("low-light check-in happened too early: %#v", decision)
		}
	}
	if !decision.Verified || decision.Rejected || !decision.NeedsAttendance ||
		decision.LivenessStatus != "低光自动识别通过" {
		t.Fatalf("uncertain low-light score should automatically pass: %#v", decision)
	}
}

func TestLowLightModeUsesRelaxedQualityFloorButKeepsNormalFloor(t *testing.T) {
	observation := recognitionObservation{Exposure: 0.25, Sharpness: 0.055}
	if livenessObservationQualityUsable(observation) {
		t.Fatal("normal mode must keep the standard exposure and sharpness floor")
	}
	observation.LowLightMode = true
	if !livenessObservationQualityUsable(observation) {
		t.Fatal("low-light mode should accept dim but still measurable frames")
	}
	if livenessMaxFramesForMode(true) <= livenessMaxFrames ||
		livenessDecisionTimeoutForMode(true) <= livenessDecisionTimeout {
		t.Fatal("low-light mode should allow additional frames and sampling time")
	}
}


func TestRecognitionTrackerLowLightBlurAutomaticallyPassesStableMatch(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 91, StudentNo: "S91", Name: "低光模糊测试"}
	var decision trackDecision
	for i := 0; i < livenessMinFrames; i++ {
		decision = tracker.Observe("camera-low-light-blur", []recognitionObservation{{
			Student: student, Similarity: 0.816, LiveScore: 0.007,
			Exposure: 0.25, Sharpness: 0.055, LowLightMode: true,
		}}, now.Add(time.Duration(i)*200*time.Millisecond))[0]
		if i < livenessMinFrames-1 && (decision.Verified || decision.Rejected) {
			t.Fatalf("blurry match decided too early: %#v", decision)
		}
	}
	if !decision.Verified || decision.Rejected || !decision.NeedsAttendance {
		t.Fatalf("blurry low-light match should automatically check in: %#v", decision)
	}
	tracker.Commit("camera-low-light-blur", decision.TrackID)
	committed := tracker.Observe("camera-low-light-blur", []recognitionObservation{{
		Student: student, Similarity: 0.82, LiveScore: 0.01,
		Exposure: 0.25, Sharpness: 0.055, LowLightMode: true,
	}}, now.Add(time.Second))[0]
	if !committed.Verified || committed.NeedsAttendance || committed.Rejected {
		t.Fatalf("committed match must not write attendance again: %#v", committed)
	}
}

func TestRecognitionTrackerLowLightTooDarkFramesStayPending(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 92, StudentNo: "S92", Name: "极暗画面测试"}
	for i := 0; i < livenessMaxFrames; i++ {
		decision := tracker.Observe("camera-low-light-too-dark", []recognitionObservation{{
			Student: student, Similarity: 0.88, LiveScore: 0.01,
			Exposure: 0.12, Sharpness: 0.04, LowLightMode: true,
		}}, now.Add(time.Duration(i)*200*time.Millisecond))[0]
		if decision.Verified || decision.Rejected || decision.NeedsAttendance || decision.Frames != 0 {
			t.Fatalf("unusable dark frames must not auto-check-in or claim spoof: %#v", decision)
		}
	}
}

func TestRecognitionTrackerLowLightClearSpoofVoteBlocksAutomaticFallback(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 94, StudentNo: "S94", Name: "混合画面测试"}
	for i := 0; i < livenessLowLightMaxFrames; i++ {
		score, sharpness := 0.55, 0.055
		if i == 0 {
			score, sharpness = 0.05, 0.18
		}
		decision := tracker.Observe("camera-low-light-mixed", []recognitionObservation{{
			Student: student, Similarity: 0.91, LiveScore: score,
			Exposure: 0.25, Sharpness: sharpness, LowLightMode: true,
		}}, now.Add(time.Duration(i)*200*time.Millisecond))[0]
		if decision.Verified || decision.NeedsAttendance {
			t.Fatalf("reliable spoof evidence must block auto-approval: %#v", decision)
		}
		if i == livenessLowLightMaxFrames-1 && !decision.TimedOut {
			t.Fatalf("ambiguous mixed evidence should end without attendance: %#v", decision)
		}
	}
}

func TestRecognitionTrackerLowLightSpoofNeedsClearFrames(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 93, StudentNo: "S93", Name: "清晰照片测试"}
	var decision trackDecision
	for i := 0; i < livenessFastFrames; i++ {
		decision = tracker.Observe("camera-low-light-clear-spoof", []recognitionObservation{{
			Student: student, Similarity: 0.93, LiveScore: 0.05,
			Exposure: 0.25, Sharpness: 0.18, LowLightMode: true,
		}}, now.Add(time.Duration(i)*180*time.Millisecond))[0]
	}
	if !decision.Rejected || decision.Verified {
		t.Fatalf("clear low-light spoof evidence must still be rejected: %#v", decision)
	}
}
