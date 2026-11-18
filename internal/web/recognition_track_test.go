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

func TestRecognitionTrackerNormalPassesThreeModerateLiveFrames(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 2, StudentNo: "S2", Name: "乙"}
	scores := []float64{0.70, 0.69, 0.67}
	var decision trackDecision
	for i, score := range scores {
		decision = tracker.Observe("camera-normal", []recognitionObservation{{
			Student: student, Similarity: 0.92, LiveScore: score, Exposure: 1, Sharpness: 1,
		}}, now.Add(time.Duration(i)*220*time.Millisecond))[0]
		if i < len(scores)-1 && decision.Verified {
			t.Fatalf("moderate track verified before normal frame %d", i+1)
		}
	}
	if !decision.Verified || decision.Frames != 3 || decision.TimedOut {
		t.Fatalf("moderate live track did not pass on frame %d: %#v", livenessMinFrames, decision)
	}
}

func TestRecognitionTrackerKeepsMultipleStudentsLivenessIndependent(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	liveStudent := store.Student{ID: 201, StudentNo: "S201", Name: "现场学生"}
	spoofStudent := store.Student{ID: 202, StudentNo: "S202", Name: "照片测试"}
	var decisions []trackDecision

	for i := 0; i < livenessMinFrames; i++ {
		decisions = tracker.Observe("camera-multi-person", []recognitionObservation{
			{Student: liveStudent, Similarity: 0.94, LiveScore: 0.90, Exposure: 1, Sharpness: 1},
			{Student: spoofStudent, Similarity: 0.93, LiveScore: 0.10, Exposure: 1, Sharpness: 1},
		}, now.Add(time.Duration(i)*180*time.Millisecond))
		if len(decisions) != 2 {
			t.Fatalf("multi-person decisions=%d want=2", len(decisions))
		}
		if i < livenessMinFrames-1 && (decisions[0].Verified || decisions[1].Rejected) {
			t.Fatalf("one student's frames affected the other before the decision point: %#v", decisions)
		}
	}

	if !decisions[0].Verified || decisions[0].Rejected {
		t.Fatalf("live student should pass independently: %#v", decisions[0])
	}
	if !decisions[1].Rejected || decisions[1].Verified {
		t.Fatalf("spoof student should be rejected independently: %#v", decisions[1])
	}
	if decisions[0].TrackID == decisions[1].TrackID {
		t.Fatalf("different students must have independent liveness tracks: %#v", decisions)
	}
}

func TestRecognitionTrackerPassesWhenTwoOfThreeFramesMeetRelaxedThreshold(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 22, StudentNo: "S22", Name: "阈值测试"}
	scores := []float64{0.50, 0.65, 0.65}
	var decision trackDecision
	for i, score := range scores {
		decision = tracker.Observe("camera-relaxed", []recognitionObservation{{
			Student: student, Similarity: 0.91, LiveScore: score, Exposure: 1, Sharpness: 1,
		}}, now.Add(time.Duration(i)*180*time.Millisecond))[0]
		if i < 2 && decision.Verified {
			t.Fatalf("track verified before all three frames: %#v", decision)
		}
	}
	if !decision.Verified || decision.Rejected || decision.TimedOut || decision.Frames != 3 {
		t.Fatalf("two acceptable frames with a 0.60 average should pass: %#v", decision)
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

func TestRecognitionTrackerRechecksPhotoWhenRealFaceAppearsWithoutLeaving(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 401, StudentNo: "S401", Name: "照片切换真人"}
	observation := recognitionObservation{
		Student: student, Similarity: 0.94, LiveScore: 0.10, Exposure: 1, Sharpness: 1,
	}
	var rejected trackDecision
	for i := 0; i < livenessMinFrames; i++ {
		rejected = tracker.Observe("camera-photo-to-live", []recognitionObservation{observation}, now.Add(time.Duration(i)*100*time.Millisecond))[0]
	}
	if !rejected.Rejected {
		t.Fatalf("expected photograph to be rejected: %#v", rejected)
	}
	stillPhoto := tracker.Observe("camera-photo-to-live", []recognitionObservation{observation}, now.Add(300*time.Millisecond))[0]
	if !stillPhoto.Rejected || stillPhoto.TrackID != rejected.TrackID {
		t.Fatalf("continuing photo must remain rejected: %#v", stillPhoto)
	}

	observation.LiveScore = 0.72
	firstLive := tracker.Observe("camera-photo-to-live", []recognitionObservation{observation}, now.Add(400*time.Millisecond))[0]
	if !firstLive.Rejected || firstLive.Verified {
		t.Fatalf("one live-like frame must not clear a spoof decision: %#v", firstLive)
	}
	secondLive := tracker.Observe("camera-photo-to-live", []recognitionObservation{observation}, now.Add(500*time.Millisecond))[0]
	if secondLive.TrackID == rejected.TrackID || secondLive.Rejected || secondLive.Verified || secondLive.Frames != 1 {
		t.Fatalf("fresh live evidence must start a separate verification cycle: %#v", secondLive)
	}
	thirdLive := tracker.Observe("camera-photo-to-live", []recognitionObservation{observation}, now.Add(600*time.Millisecond))[0]
	if thirdLive.Verified || thirdLive.Frames != 2 {
		t.Fatalf("recovered track must still collect three live frames: %#v", thirdLive)
	}
	verified := tracker.Observe("camera-photo-to-live", []recognitionObservation{observation}, now.Add(700*time.Millisecond))[0]
	if !verified.Verified || verified.Rejected || !verified.NeedsAttendance || verified.Frames != 3 {
		t.Fatalf("real student must pass after independent three-frame voting: %#v", verified)
	}
}

func TestRecognitionTrackerPhotoRejectionRequiresConsecutiveGoodRecoveryFrames(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 402}
	observation := recognitionObservation{
		Student: student, Similarity: 0.92, LiveScore: 0.08, Exposure: 1, Sharpness: 1,
	}
	for i := 0; i < 3; i++ {
		tracker.Observe("camera-recovery-quality", []recognitionObservation{observation}, now.Add(time.Duration(i)*100*time.Millisecond))
	}
	observation.LiveScore = 0.80
	first := tracker.Observe("camera-recovery-quality", []recognitionObservation{observation}, now.Add(300*time.Millisecond))[0]
	if !first.Rejected {
		t.Fatalf("first recovery candidate must remain blocked: %#v", first)
	}
	observation.Exposure = 0.1 // An unusable bright-looking score must break the streak.
	unusable := tracker.Observe("camera-recovery-quality", []recognitionObservation{observation}, now.Add(400*time.Millisecond))[0]
	if !unusable.Rejected {
		t.Fatalf("unusable frame cannot reset a spoof track: %#v", unusable)
	}
	observation.Exposure = 1
	second := tracker.Observe("camera-recovery-quality", []recognitionObservation{observation}, now.Add(500*time.Millisecond))[0]
	if !second.Rejected || second.Verified {
		t.Fatalf("recovery streak must restart after an unusable frame: %#v", second)
	}
	observation.LiveScore = 0.08
	spoof := tracker.Observe("camera-recovery-quality", []recognitionObservation{observation}, now.Add(600*time.Millisecond))[0]
	if !spoof.Rejected {
		t.Fatalf("another photo frame must restore the rejection streak: %#v", spoof)
	}
	observation.LiveScore = 0.80
	firstAgain := tracker.Observe("camera-recovery-quality", []recognitionObservation{observation}, now.Add(700*time.Millisecond))[0]
	if !firstAgain.Rejected {
		t.Fatalf("one fresh candidate after spoof must still be rejected: %#v", firstAgain)
	}
	restart := tracker.Observe("camera-recovery-quality", []recognitionObservation{observation}, now.Add(800*time.Millisecond))[0]
	if restart.Rejected || restart.Verified || restart.Frames != 1 || restart.TrackID == firstAgain.TrackID {
		t.Fatalf("two good recovery frames should reopen verification, not auto-approve: %#v", restart)
	}
}

func TestRecognitionTrackerCollectsThreeLiveFramesUnderSlowMultiPersonInference(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	students := []store.Student{{ID: 811}, {ID: 812}, {ID: 813}}
	observations := make([]recognitionObservation, len(students))
	for i, student := range students {
		observations[i] = recognitionObservation{
			Student: student, Similarity: 0.87, LiveScore: 0.998,
			FaceQuality: 0.82, Exposure: 1, Sharpness: 0.34,
		}
	}
	for frame, offset := range []time.Duration{0, 2500 * time.Millisecond, 5000 * time.Millisecond} {
		decisions := tracker.Observe("crowded-slow-camera", observations, now.Add(offset))
		for i, decision := range decisions {
			if frame < 2 && (decision.TimedOut || decision.Verified || decision.Rejected || decision.Frames != frame+1) {
				t.Fatalf("student %d lost pending evidence on slow frame %d: %#v", i, frame+1, decision)
			}
			if frame == 2 && (!decision.Verified || decision.TimedOut || decision.Rejected || decision.Frames != 3 || !decision.NeedsAttendance) {
				t.Fatalf("student %d never passed three good live frames: %#v", i, decision)
			}
		}
	}
}

func TestRecognitionTrackerRejectsPhotosUnderSlowMultiPersonInference(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	photo := recognitionObservation{
		Student: store.Student{ID: 814}, Similarity: 0.90, LiveScore: 0.05,
		FaceQuality: 0.83, Exposure: 1, Sharpness: 0.3,
	}
	for i, offset := range []time.Duration{0, 2600 * time.Millisecond, 5200 * time.Millisecond} {
		decision := tracker.Observe("crowded-photo-camera", []recognitionObservation{photo}, now.Add(offset))[0]
		if i < 2 && (decision.Verified || decision.Rejected || decision.TimedOut) {
			t.Fatalf("photo evidence decided too early on frame %d: %#v", i+1, decision)
		}
		if i == 2 && (!decision.Rejected || decision.Verified || decision.TimedOut) {
			t.Fatalf("three slow photo frames must remain blocked: %#v", decision)
		}
	}
}

func TestRecognitionTrackerDropsPendingEvidenceAfterMultipleMissingScans(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	live := recognitionObservation{
		Student: store.Student{ID: 815}, Similarity: 0.90, LiveScore: 0.98,
		Exposure: 1, Sharpness: 1,
	}
	first := tracker.Observe("crowded-leave", []recognitionObservation{live}, now)[0]
	for i := 0; i <= livenessMaxMissedFrames; i++ {
		tracker.Observe("crowded-leave", nil, now.Add(time.Duration(i+1)*100*time.Millisecond))
	}
	next := tracker.Observe("crowded-leave", []recognitionObservation{live}, now.Add(600*time.Millisecond))[0]
	if next.TrackID == first.TrackID || next.Frames != 1 || next.Verified {
		t.Fatalf("old PAD evidence cannot be reused after a person disappears: first=%#v next=%#v", first, next)
	}
}

func TestRecognitionTrackerKeepsShortTimeoutForFramesWithNoUsableEvidence(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	bad := recognitionObservation{
		Student: store.Student{ID: 816}, Similarity: 0.90, LiveScore: 0.99,
		Exposure: 0.05, Sharpness: 0.02,
	}
	tracker.Observe("crowded-unusable", []recognitionObservation{bad}, now)
	decision := tracker.Observe("crowded-unusable", []recognitionObservation{bad}, now.Add(livenessDecisionTimeout+100*time.Millisecond))[0]
	if !decision.TimedOut || decision.Verified || decision.Frames != 0 {
		t.Fatalf("unusable frames must still expire promptly: %#v", decision)
	}
}

func TestRecognitionTrackerTimesOutWithinTwoSecondsAndResets(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 4, StudentNo: "S4", Name: "丁"}
	var decision trackDecision
	for i, offset := range []time.Duration{0, 300 * time.Millisecond, 600 * time.Millisecond} {
		decision = tracker.Observe("camera-timeout", []recognitionObservation{{
			Student: student, Similarity: 0.91, LiveScore: 0.55, Exposure: 1, Sharpness: 1,
		}}, now.Add(offset))[0]
		if i < 2 && decision.TimedOut {
			t.Fatalf("track timed out too early at observation %d: %#v", i+1, decision)
		}
	}
	if !decision.TimedOut || decision.Verified || decision.Rejected || decision.LivenessStatus != "活体超时，请重新对准" {
		t.Fatalf("uncertain track did not time out cleanly: %#v", decision)
	}

	next := tracker.Observe("camera-timeout", []recognitionObservation{{
		Student: student, Similarity: 0.91, LiveScore: 0.90, Exposure: 1, Sharpness: 1,
	}}, now.Add(700*time.Millisecond))[0]
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
			Student: student, Similarity: 0.90, LiveScore: 0.50, Exposure: 1, Sharpness: 1,
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
	}}, now.Add(livenessPendingTrackTTL+time.Second))[0]
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
		if decision.LivenessStatus != "人脸区域曝光不足且清晰度不足" {
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

func TestRecognitionTrackerLowLightUncertainScoresDoNotAutoPass(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 90, StudentNo: "S90", Name: "夜间自动签到测试"}
	var decision trackDecision
	for i := 0; i < livenessLowLightMaxFrames; i++ {
		decision = tracker.Observe("camera-low-light-uncertain", []recognitionObservation{{
			Student: student, Similarity: 0.90, LiveScore: 0.55,
			Exposure: 0.25, Sharpness: 0.055, LowLightMode: true,
		}}, now.Add(time.Duration(i)*250*time.Millisecond))[0]
		if i < livenessLowLightMaxFrames-1 && (decision.Verified || decision.TimedOut) {
			t.Fatalf("uncertain low-light track decided before the frame limit: %#v", decision)
		}
	}
	if decision.Verified || decision.Rejected || decision.NeedsAttendance || !decision.TimedOut {
		t.Fatalf("uncertain low-light score must not pass without liveness evidence: %#v", decision)
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
	if livenessMaxFramesForMode(true) != 3 || livenessMaxFramesForMode(false) != 3 ||
		livenessDecisionTimeoutForMode(true) != livenessDecisionTimeout {
		t.Fatal("low-light mode must use the same three-frame verification limit")
	}
}


func TestRecognitionTrackerLowLightBlurDoesNotAutoPass(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 91, StudentNo: "S91", Name: "低光模糊测试"}
	var decision trackDecision
	for i := 0; i < livenessLowLightMaxFrames; i++ {
		decision = tracker.Observe("camera-low-light-blur", []recognitionObservation{{
			Student: student, Similarity: 0.816, LiveScore: 0.007,
			Exposure: 0.25, Sharpness: 0.055, LowLightMode: true,
		}}, now.Add(time.Duration(i)*200*time.Millisecond))[0]
		if i < livenessLowLightMaxFrames-1 && (decision.Verified || decision.Rejected || decision.TimedOut) {
			t.Fatalf("blurry low-light track decided before the frame limit: %#v", decision)
		}
	}
	if decision.Verified || decision.Rejected || decision.NeedsAttendance || !decision.TimedOut {
		t.Fatalf("blurry low-light match must not bypass enabled liveness: %#v", decision)
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

func TestRecognitionTrackerResetsWhenLivenessIsReEnabled(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 101, StudentNo: "S101", Name: "开关切换测试"}

	tracker.SetLivenessMode("camera-toggle", true)
	tracker.CommitDirect("camera-toggle", student.ID, now)
	disabled := tracker.Observe("camera-toggle", []recognitionObservation{{
		Student: student, Similarity: 0.95, LiveScore: 0.01, Exposure: 1, Sharpness: 1,
	}}, now.Add(100*time.Millisecond))[0]
	if !disabled.Verified || disabled.NeedsAttendance || disabled.LivenessStatus != "活体检测已关闭" {
		t.Fatalf("disabled mode should keep direct check-in committed: %#v", disabled)
	}

	tracker.SetLivenessMode("camera-toggle", false)
	reenabled := tracker.Observe("camera-toggle", []recognitionObservation{{
		Student: student, Similarity: 0.95, LiveScore: 0.01, Exposure: 1, Sharpness: 1,
	}}, now.Add(200*time.Millisecond))[0]
	if reenabled.Verified || reenabled.Rejected || reenabled.Frames != 1 ||
		reenabled.LivenessStatus != "活体验证中" {
		t.Fatalf("re-enabling liveness must discard the disabled-mode pass and start fresh verification: %#v", reenabled)
	}
}

func TestRecognitionTrackerResetsPendingFramesWhenLivenessIsDisabled(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 102, StudentNo: "S102", Name: "反向开关测试"}

	tracker.SetLivenessMode("camera-toggle-reverse", false)
	first := tracker.Observe("camera-toggle-reverse", []recognitionObservation{{
		Student: student, Similarity: 0.95, LiveScore: 0.70, Exposure: 1, Sharpness: 1,
	}}, now)[0]
	if first.Frames != 1 || first.Verified {
		t.Fatalf("first enabled frame should remain pending: %#v", first)
	}

	tracker.SetLivenessMode("camera-toggle-reverse", true)
	tracker.CommitDirect("camera-toggle-reverse", student.ID, now.Add(100*time.Millisecond))
	disabled := tracker.Observe("camera-toggle-reverse", []recognitionObservation{{
		Student: student, Similarity: 0.95, LiveScore: 0.01, Exposure: 1, Sharpness: 1,
	}}, now.Add(200*time.Millisecond))[0]
	if !disabled.Verified || disabled.NeedsAttendance || disabled.LivenessStatus != "活体检测已关闭" {
		t.Fatalf("disabling liveness should use the direct-check-in state: %#v", disabled)
	}
}


func TestRecognitionTrackerInferenceFailureStopsAfterThreeAttemptsAndResets(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 310, StudentNo: "S310", Name: "推理失败测试"}
	var decision trackDecision
	for i := 0; i < 3; i++ {
		decision = tracker.Observe("camera-inference-error", []recognitionObservation{{
			Student: student, Similarity: 0.93, Exposure: 1, Sharpness: 1,
			InferenceFailed: true,
		}}, now.Add(time.Duration(i)*200*time.Millisecond))[0]
		if i < 2 && (decision.TimedOut || decision.LivenessStatus != "活体模型推理失败，正在重试") {
			t.Fatalf("inference failure should retry before terminal failure: %#v", decision)
		}
	}
	if !decision.TimedOut || decision.LivenessStatus != "活体检测失败，请检查模型和运行日志" {
		t.Fatalf("repeated inference failures did not reach a terminal state: %#v", decision)
	}
	next := tracker.Observe("camera-inference-error", []recognitionObservation{{
		Student: student, Similarity: 0.93, LiveScore: 0.9, Exposure: 1, Sharpness: 1,
	}}, now.Add(time.Second))[0]
	if next.TimedOut || next.Frames != 1 || next.TrackID == decision.TrackID {
		t.Fatalf("successful retry did not start a fresh liveness cycle: %#v", next)
	}
}

func TestRecognitionTrackerPoorQualityFramesEventuallyTimeoutWithoutResettingClock(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 311, StudentNo: "S311", Name: "低质量超时测试"}
	var decision trackDecision
	for i := 0; i < 7; i++ {
		decision = tracker.Observe("camera-poor-quality-timeout", []recognitionObservation{{
			Student: student, Similarity: 0.92, LiveScore: 0.9,
			Exposure: 0.10, Sharpness: 0.02,
		}}, now.Add(time.Duration(i)*200*time.Millisecond))[0]
	}
	if !decision.TimedOut || decision.LivenessStatus != "光线不足或画面模糊，活体检测超时" {
		t.Fatalf("continuous poor-quality frames should end instead of remaining pending forever: %#v", decision)
	}
}

func TestLivenessQualityGateAdaptsSharpnessForStrongFaceQuality(t *testing.T) {
	observation := recognitionObservation{
		FaceQuality: 0.72,
		Exposure:    0.90,
		Sharpness:   0.06,
	}
	if !livenessObservationQualityUsable(observation) {
		t.Fatal("high-quality face should use the existing relaxed sharpness floor")
	}
	observation.FaceQuality = 0.69
	if livenessObservationQualityUsable(observation) {
		t.Fatal("ordinary face quality must keep the normal sharpness floor")
	}
	observation.FaceQuality = 0.72
	observation.Sharpness = 0.04
	if livenessObservationQualityUsable(observation) {
		t.Fatal("adaptive quality must not accept severely blurred frames")
	}
}

func TestLivenessObservationQualityStatusIdentifiesBlockingDimension(t *testing.T) {
	observation := recognitionObservation{FaceQuality: 0.72, Exposure: 0.20, Sharpness: 0.04}
	if got := livenessObservationQualityStatus(observation); got != "人脸区域曝光不足且清晰度不足" {
		t.Fatalf("unexpected combined quality status: %q", got)
	}
	observation.Exposure = 0.90
	if got := livenessObservationQualityStatus(observation); got != "人脸区域清晰度不足" {
		t.Fatalf("unexpected sharpness status: %q", got)
	}
}

func TestAutoLowLightProfileAcceptsDimButMeasurableFaceRegion(t *testing.T) {
	lowLight, _ := resolveLivenessMode(false, 24)
	observation := recognitionObservation{
		FaceQuality: 0.67,
		FaceBrightness: 24,
		Exposure: 0.51,
		Sharpness: 0.055,
		LowLightMode: lowLight,
	}
	if !lowLight || !livenessObservationQualityUsable(observation) {
		t.Fatalf("measurable dim face should enter anti-spoof inference and frame accumulation: %#v", observation)
	}
	if livenessObservationQualityStatus(observation) != "画面质量达标，正在累计活体帧" {
		t.Fatalf("unexpected adaptive low-light status: %s", livenessObservationQualityStatus(observation))
	}
}

func TestAutoLowLightProfileDoesNotAcceptExtremelyDarkOrBlurredFrames(t *testing.T) {
	lowLight, _ := resolveLivenessMode(false, 12)
	observation := recognitionObservation{
		FaceQuality: 0.67,
		FaceBrightness: 12,
		Exposure: 0.12,
		Sharpness: 0.02,
		LowLightMode: lowLight,
	}
	if !lowLight || livenessObservationQualityUsable(observation) {
		t.Fatalf("extremely dark and blurred face must remain blocked: %#v", observation)
	}
	if livenessObservationQualityStatus(observation) != "人脸区域曝光不足且清晰度不足" {
		t.Fatalf("unexpected blocked status: %s", livenessObservationQualityStatus(observation))
	}
}

func TestAutoLowLightProfileStillRequiresThreePositiveAntiSpoofFrames(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 312, StudentNo: "S312", Name: "自动低光活体测试"}
	lowLight, _ := resolveLivenessMode(false, 24)
	var decision trackDecision
	for i := 0; i < livenessMinFrames; i++ {
		decision = tracker.Observe("camera-auto-low-light-live", []recognitionObservation{{
			Student: student, Similarity: 0.91, LiveScore: 0.70,
			FaceQuality: 0.67, FaceBrightness: 24, Exposure: 0.51,
			Sharpness: 0.055, LowLightMode: lowLight,
		}}, now.Add(time.Duration(i)*220*time.Millisecond))[0]
		if i < livenessMinFrames-1 && decision.Verified {
			t.Fatalf("auto low-light mode passed before three anti-spoof frames: %#v", decision)
		}
	}
	if !decision.Verified || decision.Rejected || decision.Frames != 3 {
		t.Fatalf("three positive anti-spoof frames should pass in auto low-light mode: %#v", decision)
	}
}
