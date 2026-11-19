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

func TestRecognitionTrackerIgnoresOneMotionBlurOutlier(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 203, StudentNo: "S203", Name: "运动真人"}
	scores := []float64{0.90, 0.18, 0.82}
	var decision trackDecision
	for i, score := range scores {
		decision = tracker.Observe("camera-motion", []recognitionObservation{{
			Student: student, Similarity: 0.93, LiveScore: score, Exposure: 1, Sharpness: 1,
		}}, now.Add(time.Duration(i)*120*time.Millisecond))[0]
	}
	if !decision.Verified || decision.Rejected || decision.Frames != 3 {
		t.Fatalf("one motion-blurred outlier should not block a real face: %#v", decision)
	}
	if len(decision.Scores) != 3 || decision.Scores[1] != 0.18 {
		t.Fatalf("raw frame scores were not preserved: %#v", decision.Scores)
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

func TestRecognitionTrackerPhotoRejectionToleratesOneRecoveryOutlier(t *testing.T) {
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
	observation.LiveScore = 0.08 // One motion/blurred frame is tolerated during recovery.
	stillBlocked := tracker.Observe("camera-recovery-quality", []recognitionObservation{observation}, now.Add(400*time.Millisecond))[0]
	if !stillBlocked.Rejected {
		t.Fatalf("one low model score must keep the attack blocked: %#v", stillBlocked)
	}
	observation.LiveScore = 0.80
	restart := tracker.Observe("camera-recovery-quality", []recognitionObservation{observation}, now.Add(500*time.Millisecond))[0]
	if restart.Rejected || restart.Verified || restart.Frames != 1 || restart.TrackID == first.TrackID {
		t.Fatalf("two good recovery frames in a three-frame window should reopen verification, not auto-approve: %#v", restart)
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

func TestRecognitionTrackerDoesNotTimeoutOnImageQualityMetrics(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	observation := recognitionObservation{
		Student: store.Student{ID: 816}, Similarity: 0.90, LiveScore: 0.99,
		Exposure: 0.05, Sharpness: 0.02,
	}
	first := tracker.Observe("crowded-unusable", []recognitionObservation{observation}, now)[0]
	if first.Frames != 1 || first.TimedOut || first.Verified {
		t.Fatalf("low image-quality metrics must not block the first model score: %#v", first)
	}
	second := tracker.Observe("crowded-unusable", []recognitionObservation{observation}, now.Add(livenessDecisionTimeout+100*time.Millisecond))[0]
	if second.Frames != 2 || second.TimedOut || second.Verified {
		t.Fatalf("collected model scores must use the bounded multi-frame window: %#v", second)
	}
	third := tracker.Observe("crowded-unusable", []recognitionObservation{observation}, now.Add(2*(livenessDecisionTimeout+100*time.Millisecond)))[0]
	if !third.Verified || third.Frames != 3 {
		t.Fatalf("three positive model scores should pass regardless of diagnostic image metrics: %#v", third)
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


func TestRecognitionTrackerUsesModelScoresWithoutLowLightQualityGate(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 77, StudentNo: "S77", Name: "暗光真人"}
	var decision trackDecision
	for i := 0; i < livenessMinFrames; i++ {
		decision = tracker.Observe("camera-dark-live", []recognitionObservation{{
			Student: student, Similarity: 0.93, LiveScore: 0.90,
			FaceQuality: 0.68, FaceBrightness: 7, Exposure: 0.52, Sharpness: 0,
		}}, now.Add(time.Duration(i)*180*time.Millisecond))[0]
		if i < livenessMinFrames-1 && decision.Verified {
			t.Fatalf("dark-scene model scores passed before three frames: %#v", decision)
		}
	}
	if !decision.Verified || decision.Rejected || decision.Frames != 3 {
		t.Fatalf("positive model scores must accumulate despite low-light diagnostics: %#v", decision)
	}
}

func TestRecognitionTrackerRejectsLowModelScoresRegardlessOfImageQuality(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 78, StudentNo: "S78", Name: "暗光低分测试"}
	var decision trackDecision
	for i := 0; i < livenessMinFrames; i++ {
		decision = tracker.Observe("camera-dark-low-score", []recognitionObservation{{
			Student: student, Similarity: 0.894, LiveScore: 0.026,
			FaceQuality: 0.68, FaceBrightness: 7, Exposure: 0.52, Sharpness: 0,
		}}, now.Add(time.Duration(i)*180*time.Millisecond))[0]
		if i < livenessMinFrames-1 && decision.Rejected {
			t.Fatalf("low model score was rejected before enough frames: %#v", decision)
		}
	}
	if !decision.Rejected || decision.Verified || decision.Frames != 3 {
		t.Fatalf("three low MiniFAS scores must be rejected even when quality metrics are low: %#v", decision)
	}
}

func TestRecognitionTrackerUncertainModelScoresNeverAutoPass(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 79, StudentNo: "S79", Name: "不确定分数测试"}
	var decision trackDecision
	for i := 0; i < livenessMaxFrames; i++ {
		decision = tracker.Observe("camera-uncertain-score", []recognitionObservation{{
			Student: student, Similarity: 0.90, LiveScore: 0.55,
		}}, now.Add(time.Duration(i)*180*time.Millisecond))[0]
	}
	if decision.Verified || decision.Rejected || decision.NeedsAttendance || !decision.TimedOut {
		t.Fatalf("uncertain model scores must time out without attendance: %#v", decision)
	}
}

func TestRecognitionTrackerPhotoRecoveryUsesOnlyModelEvidence(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 80, StudentNo: "S80", Name: "照片切换真人"}
	photo := recognitionObservation{Student: student, Similarity: 0.94, LiveScore: 0.05, Exposure: 0.1, Sharpness: 0}
	var decision trackDecision
	for i := 0; i < livenessMinFrames; i++ {
		decision = tracker.Observe("camera-photo-live-recovery", []recognitionObservation{photo}, now.Add(time.Duration(i)*180*time.Millisecond))[0]
	}
	if !decision.Rejected {
		t.Fatalf("photo should be rejected first: %#v", decision)
	}
	live := photo
	live.LiveScore = 0.90
	first := tracker.Observe("camera-photo-live-recovery", []recognitionObservation{live}, now.Add(600*time.Millisecond))[0]
	if !first.Rejected {
		t.Fatalf("first live score must not clear attack: %#v", first)
	}
	second := tracker.Observe("camera-photo-live-recovery", []recognitionObservation{live}, now.Add(780*time.Millisecond))[0]
	if second.Rejected || second.Verified || second.Frames != 1 {
		t.Fatalf("second live score must start a fresh three-frame cycle: %#v", second)
	}
	decision = second
	for i := 0; i < 2; i++ {
		decision = tracker.Observe("camera-photo-live-recovery", []recognitionObservation{live}, now.Add(time.Duration(960+i*180)*time.Millisecond))[0]
	}
	if !decision.Verified || decision.Rejected || decision.Frames != 3 {
		t.Fatalf("real person should pass only after a fresh three-frame model vote: %#v", decision)
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

func TestRecognitionTrackerInferenceFailuresEventuallyTimeout(t *testing.T) {
	tracker := newRecognitionTracker()
	now := time.Now()
	student := store.Student{ID: 311, StudentNo: "S311", Name: "模型推理失败测试"}
	var decision trackDecision
	for i := 0; i < 3; i++ {
		decision = tracker.Observe("camera-inference-failure", []recognitionObservation{{
			Student: student, Similarity: 0.92, InferenceFailed: true,
		}}, now.Add(time.Duration(i)*200*time.Millisecond))[0]
	}
	if !decision.TimedOut || decision.Verified || decision.NeedsAttendance {
		t.Fatalf("repeated inference failures must stop without attendance: %#v", decision)
	}
}
