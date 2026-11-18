package web

import (
	"fmt"
	"sync"
	"time"

	"github.com/hwyc888/FaceSign/internal/store"
)

const (
	livenessTrackTTL          = 3 * time.Second
	// Multi-person CPU inference can take seconds between valid snapshots.
	// Keep pending evidence briefly, but never indefinitely or across long absences.
	livenessPendingTrackTTL   = 6 * time.Second
	livenessCollectionTimeout = 12 * time.Second
	livenessMaxMissedFrames    = 3
	livenessFastFrames        = 3
	livenessMinFrames         = 3
	livenessMaxFrames         = 3
	livenessFrameThreshold    = 0.55
	livenessFastPassThreshold = 0.80
	livenessPassThreshold     = 0.60
	livenessSpoofThreshold    = 0.40
	livenessDecisionTimeout   = 1200 * time.Millisecond
	livenessMinExposureScore  = 0.35
	livenessMinSharpnessScore = 0.08
	// A strong overall face-quality score permits the same sharpness floor
	// already used by low-light mode, while exposure and spoof checks remain.
	livenessAdaptiveFaceQuality = 0.70
	livenessAdaptiveSharpness   = 0.05

	livenessLowLightMaxFrames        = 3
	livenessLowLightDecisionTimeout = 1200 * time.Millisecond
	livenessLowLightMinExposureScore = 0.20
	livenessLowLightMinSharpnessScore = 0.05
	// Automatically use the low-light quality profile for dark inner-face regions.
	livenessAutoLowLightBrightness = 75
	// Blurry low-light frames cannot reliably prove a photo/screen attack.
	livenessLowLightSpoofMinSharpnessScore = 0.12
)

type recognitionObservation struct {
	Student    store.Student
	Similarity float64
	LiveScore  float64
	FaceQuality float64
	Exposure    float64
	Sharpness      float64
	FaceBrightness float64
	LowLightMode   bool
	InferenceFailed bool
}

type recognitionTrack struct {
	ID               string
	FirstSeen        time.Time
	LastSeen         time.Time
	Frames           int
	MissedFrames     int
	Scores           []float64
	SimilaritySum    float64
	Verified         bool
	Rejected         bool
	Committed        bool
	LowLightMode     bool
	LivenessDisabled bool
	SpoofVotes        int
	RecoveryVotes     int
	InferenceFailures int
}

type trackDecision struct {
	TrackID         string
	Frames          int
	RequiredFrames  int
	LiveScore       float64
	Similarity      float64
	Verified        bool
	Rejected        bool
	TimedOut        bool
	NeedsAttendance bool
	LivenessStatus  string
}

type recognitionTrackSession struct {
	Tracks                 map[int64]*recognitionTrack
	NextID                 uint64
	LivenessModeSet        bool
	LivenessDisabled       bool
}

type recognitionTracker struct {
	mu       sync.Mutex
	sessions map[string]*recognitionTrackSession
}

func newRecognitionTracker() *recognitionTracker {
	return &recognitionTracker{sessions: make(map[string]*recognitionTrackSession)}
}

// SetLivenessMode starts a fresh verification cycle whenever the session setting changes.
func (t *recognitionTracker) SetLivenessMode(sessionID string, disabled bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if sessionID == "" {
		sessionID = "default"
	}
	session := t.sessions[sessionID]
	if session == nil {
		session = &recognitionTrackSession{Tracks: make(map[int64]*recognitionTrack)}
		t.sessions[sessionID] = session
	}
	if session.LivenessModeSet && session.LivenessDisabled != disabled {
		session.Tracks = make(map[int64]*recognitionTrack)
	}
	session.LivenessModeSet = true
	session.LivenessDisabled = disabled
}

func (t *recognitionTracker) Observe(sessionID string, observations []recognitionObservation, now time.Time) []trackDecision {
	t.mu.Lock()
	defer t.mu.Unlock()

	if sessionID == "" {
		sessionID = "default"
	}
	session := t.sessions[sessionID]
	if session == nil {
		session = &recognitionTrackSession{Tracks: make(map[int64]*recognitionTrack)}
		t.sessions[sessionID] = session
	}

	for studentID, track := range session.Tracks {
		ttl := livenessTrackTTL
		if !track.Verified && !track.Rejected && !track.Committed && len(track.Scores) > 0 {
			ttl = livenessPendingTrackTTL
		}
		if now.Sub(track.LastSeen) > ttl {
			delete(session.Tracks, studentID)
			continue
		}
		// Do not combine old and new face evidence across several scans where
		// the student was no longer matched, even if each scan was slow.
		if !track.Committed {
			track.MissedFrames++
			if track.MissedFrames > livenessMaxMissedFrames {
				delete(session.Tracks, studentID)
			}
		}
	}

	out := make([]trackDecision, len(observations))
	for i, observation := range observations {
		track := session.Tracks[observation.Student.ID]
		if track != nil {
			track.MissedFrames = 0
		}
		if track != nil && track.Committed {
			out[i] = trackDecision{
				TrackID:        track.ID,
				Verified:       track.Verified,
				LivenessStatus: livenessVerifiedStatus(track),
			}
			continue
		}

		if track == nil {
			session.NextID++
			track = &recognitionTrack{
				ID:        fmt.Sprintf("%s-%d", sessionID, session.NextID),
				FirstSeen: now,
				Scores:    make([]float64, 0, livenessMaxFramesForMode(observation.LowLightMode)),
				LowLightMode: observation.LowLightMode,
			}
			session.Tracks[observation.Student.ID] = track
		}
		track.LastSeen = now
		if track.Rejected {
			// Keep rejecting a photo until two consecutive usable frames indicate
			// fresh live evidence. Never carry rejected votes into a new cycle.
			if !observation.InferenceFailed &&
				livenessObservationQualityUsable(observation) &&
				observation.LiveScore >= livenessPassThreshold {
				track.RecoveryVotes++
			} else {
				track.RecoveryVotes = 0
			}
			if track.RecoveryVotes < 2 {
				out[i] = trackDecision{
					TrackID:        track.ID,
					Frames:         len(track.Scores),
					RequiredFrames: livenessMinFrames,
					LiveScore:      averageScores(track.Scores),
					Rejected:       true,
					LivenessStatus: "疑似照片/屏幕",
				}
				continue
			}
			// The scene may change from a photo to the real student without the
			// student ID or person box changing. Start independent PAD voting.
			session.NextID++
			track = &recognitionTrack{
				ID:           fmt.Sprintf("%s-%d", sessionID, session.NextID),
				FirstSeen:    now,
				Scores:       make([]float64, 0, livenessMaxFramesForMode(observation.LowLightMode)),
				LowLightMode: observation.LowLightMode,
			}
			session.Tracks[observation.Student.ID] = track
		}
		track.LastSeen = now
		track.LowLightMode = observation.LowLightMode
		decisionTimeout := livenessDecisionTimeoutForMode(track.LowLightMode)
		if len(track.Scores) > 0 {
			// The short deadline is for a frame with no usable evidence. Once
			// positive/negative PAD scores are being collected, allow a bounded
			// multi-person interval before resetting the three-frame vote.
			decisionTimeout = livenessCollectionTimeout
		}

		if observation.InferenceFailed {
			track.InferenceFailures++
			timedOut := track.InferenceFailures >= 3 || now.Sub(track.FirstSeen) >= decisionTimeout
			status := "活体模型推理失败，正在重试"
			if timedOut {
				status = "活体检测失败，请检查模型和运行日志"
			}
			out[i] = trackDecision{
				TrackID:        track.ID,
				Frames:         len(track.Scores),
				RequiredFrames: livenessFastFrames,
				LiveScore:      averageScores(track.Scores),
				Similarity:     track.SimilaritySum / maxLivenessFloat(1, float64(track.Frames)),
				TimedOut:       timedOut,
				LivenessStatus: status,
			}
			if timedOut {
				delete(session.Tracks, observation.Student.ID)
			}
			continue
		}

		track.InferenceFailures = 0
		if !livenessObservationQualityUsable(observation) {
			status := livenessObservationQualityStatus(observation)
			timedOut := !track.Verified && !track.Rejected && now.Sub(track.FirstSeen) >= decisionTimeout
			if track.Verified {
				status = livenessVerifiedStatus(track)
			} else if track.Rejected {
				status = "疑似照片/屏幕"
			} else if timedOut {
				status = "光线不足或画面模糊，活体检测超时"
			}
			out[i] = trackDecision{
				TrackID:         track.ID,
				Frames:          len(track.Scores),
				RequiredFrames:  requiredLivenessFrames(track),
				LiveScore:       averageScores(track.Scores),
				Similarity:      track.SimilaritySum / maxLivenessFloat(1, float64(track.Frames)),
				Verified:        track.Verified,
				Rejected:        track.Rejected,
				TimedOut:        timedOut,
				NeedsAttendance: track.Verified && !track.Committed,
				LivenessStatus:  status,
			}
			if timedOut {
				delete(session.Tracks, observation.Student.ID)
			}
			continue
		}
		maxFrames := livenessMaxFramesForMode(track.LowLightMode)
		track.LastSeen = now
		track.Frames++
		track.SimilaritySum += observation.Similarity
		if observation.LiveScore < livenessSpoofThreshold && livenessObservationSpoofQualityUsable(observation) {
			track.SpoofVotes++
		}
		track.Scores = append(track.Scores, observation.LiveScore)
		if len(track.Scores) > maxFrames {
			track.Scores = append([]float64(nil), track.Scores[len(track.Scores)-maxFrames:]...)
		}

		average := averageScores(track.Scores)
		liveVotes := 0
		for _, score := range track.Scores {
			if score >= livenessFrameThreshold {
				liveVotes++
			}
		}

		timedOut := false
		if !track.Verified && !track.Rejected {
			switch {
			case len(track.Scores) >= livenessFastFrames &&
				average < livenessSpoofThreshold && track.SpoofVotes >= 2:
				track.Rejected = true
			case len(track.Scores) >= livenessMinFrames &&
				average >= livenessPassThreshold &&
				liveVotes >= 2:
				track.Verified = true
			case now.Sub(track.FirstSeen) >= decisionTimeout || len(track.Scores) >= maxFrames:
				timedOut = true
			}
		}
		requiredFrames := requiredLivenessFrames(track)
		if timedOut {
			requiredFrames = maxFrames
		}

		status := "活体验证中"
		switch {
		case track.Verified:
			status = livenessVerifiedStatus(track)
		case track.Rejected:
			status = "疑似照片/屏幕"
		case timedOut:
			status = "活体超时，请重新对准"
		case len(track.Scores) >= livenessFastFrames:
			status = "活体复核中"
		}

		out[i] = trackDecision{
			TrackID:         track.ID,
			Frames:          len(track.Scores),
			RequiredFrames:  requiredFrames,
			LiveScore:       average,
			Similarity:      track.SimilaritySum / float64(track.Frames),
			Verified:        track.Verified,
			Rejected:        track.Rejected,
			TimedOut:        timedOut,
			NeedsAttendance: track.Verified && !track.Committed,
			LivenessStatus:  status,
		}
		if timedOut {
			delete(session.Tracks, observation.Student.ID)
		}
	}
	return out
}

func (t *recognitionTracker) IsCommitted(sessionID string, studentID int64, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if sessionID == "" {
		sessionID = "default"
	}
	session := t.sessions[sessionID]
	if session == nil {
		return false
	}
	track := session.Tracks[studentID]
	if track == nil {
		return false
	}
	if now.Sub(track.LastSeen) > livenessTrackTTL {
		delete(session.Tracks, studentID)
		return false
	}
	track.LastSeen = now
	return track.Committed
}

func (t *recognitionTracker) CommitDirect(sessionID string, studentID int64, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if sessionID == "" {
		sessionID = "default"
	}
	session := t.sessions[sessionID]
	if session == nil {
		session = &recognitionTrackSession{Tracks: make(map[int64]*recognitionTrack)}
		t.sessions[sessionID] = session
	}
	track := session.Tracks[studentID]
	if track == nil {
		session.NextID++
		track = &recognitionTrack{
			ID:        fmt.Sprintf("%s-%d", sessionID, session.NextID),
			FirstSeen: now,
		}
		session.Tracks[studentID] = track
	}
	track.Verified = true
	track.Committed = true
	track.LivenessDisabled = true
	track.LastSeen = now
}

func (t *recognitionTracker) Commit(sessionID, trackID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	session := t.sessions[sessionID]
	if session == nil {
		return
	}
	for _, track := range session.Tracks {
		if track.ID == trackID {
			track.Committed = true
			return
		}
	}
}

func livenessQualityThresholds(observation recognitionObservation) (float64, float64) {
	if observation.LowLightMode {
		return livenessLowLightMinExposureScore, livenessLowLightMinSharpnessScore
	}
	sharpnessThreshold := livenessMinSharpnessScore
	if observation.FaceQuality >= livenessAdaptiveFaceQuality {
		// Identity recognition already accepted a clear, high-quality face.
		// Relax only the sharpness gate to the existing low-light floor.
		sharpnessThreshold = livenessAdaptiveSharpness
	}
	return livenessMinExposureScore, sharpnessThreshold
}

func livenessObservationQualityUsable(observation recognitionObservation) bool {
	exposureThreshold, sharpnessThreshold := livenessQualityThresholds(observation)
	return observation.Exposure >= exposureThreshold &&
		observation.Sharpness >= sharpnessThreshold
}

func livenessObservationQualityStatus(observation recognitionObservation) string {
	exposureThreshold, sharpnessThreshold := livenessQualityThresholds(observation)
	exposureLow := observation.Exposure < exposureThreshold
	sharpnessLow := observation.Sharpness < sharpnessThreshold
	switch {
	case exposureLow && sharpnessLow:
		return "人脸区域曝光不足且清晰度不足"
	case exposureLow:
		return "人脸区域曝光不足"
	case sharpnessLow:
		return "人脸区域清晰度不足"
	default:
		return "画面质量达标，正在累计活体帧"
	}
}

func livenessObservationSpoofQualityUsable(observation recognitionObservation) bool {
	if !observation.LowLightMode {
		return true
	}
	return observation.Exposure >= livenessLowLightMinExposureScore &&
		observation.Sharpness >= livenessLowLightSpoofMinSharpnessScore
}

func livenessVerifiedStatus(track *recognitionTrack) string {
	if track.LivenessDisabled {
		return "活体检测已关闭"
	}
	return "活体通过"
}

func livenessMaxFramesForMode(lowLightMode bool) int {
	if lowLightMode {
		return livenessLowLightMaxFrames
	}
	return livenessMaxFrames
}

func livenessDecisionTimeoutForMode(lowLightMode bool) time.Duration {
	if lowLightMode {
		return livenessLowLightDecisionTimeout
	}
	return livenessDecisionTimeout
}

func requiredLivenessFrames(track *recognitionTrack) int {
	return livenessMinFrames
}

func maxLivenessFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func averageScores(scores []float64) float64 {
	if len(scores) == 0 {
		return 0
	}
	var sum float64
	for _, score := range scores {
		sum += score
	}
	return sum / float64(len(scores))
}
