package web

import (
	"fmt"
	"sync"
	"time"

	"github.com/hwyc888/FaceSign/internal/store"
)

const (
	livenessTrackTTL          = 3 * time.Second
	livenessFastFrames        = 3
	livenessMinFrames         = 3
	livenessMaxFrames         = 3
	livenessFrameThreshold    = 0.60
	livenessFastPassThreshold = 0.80
	livenessPassThreshold     = 0.68
	livenessSpoofThreshold    = 0.40
	livenessDecisionTimeout   = 1200 * time.Millisecond
	livenessMinExposureScore  = 0.35
	livenessMinSharpnessScore = 0.08

	livenessLowLightMaxFrames        = 3
	livenessLowLightDecisionTimeout = 1200 * time.Millisecond
	livenessLowLightMinExposureScore = 0.20
	livenessLowLightMinSharpnessScore = 0.05
	// Blurry low-light frames cannot reliably prove a photo/screen attack.
	livenessLowLightSpoofMinSharpnessScore = 0.12
)

type recognitionObservation struct {
	Student    store.Student
	Similarity float64
	LiveScore  float64
	Exposure   float64
	Sharpness    float64
	LowLightMode bool
}

type recognitionTrack struct {
	ID               string
	FirstSeen        time.Time
	LastSeen         time.Time
	Frames           int
	Scores           []float64
	SimilaritySum    float64
	Verified         bool
	Rejected         bool
	Committed        bool
	LowLightMode     bool
	LivenessDisabled bool
	SpoofVotes        int
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
		if now.Sub(track.LastSeen) > livenessTrackTTL {
			delete(session.Tracks, studentID)
		}
	}

	out := make([]trackDecision, len(observations))
	for i, observation := range observations {
		track := session.Tracks[observation.Student.ID]
		if track != nil && track.Committed {
			out[i] = trackDecision{
				TrackID:        track.ID,
				Verified:       track.Verified,
				LivenessStatus: livenessVerifiedStatus(track),
			}
			continue
		}
		if !livenessObservationQualityUsable(observation) {
			status := "光线不足或画面模糊，请调整摄像头"
			if track == nil {
				out[i] = trackDecision{
					RequiredFrames: livenessFastFrames,
					LivenessStatus: status,
				}
				continue
			}
			track.LastSeen = now
			if !track.Verified && !track.Rejected {
				track.FirstSeen = now
			}
			if track.Verified {
				status = livenessVerifiedStatus(track)
			} else if track.Rejected {
				status = "疑似照片/屏幕"
			}
			out[i] = trackDecision{
				TrackID:         track.ID,
				Frames:          len(track.Scores),
				RequiredFrames:  requiredLivenessFrames(track),
				LiveScore:       averageScores(track.Scores),
				Similarity:      track.SimilaritySum / maxLivenessFloat(1, float64(track.Frames)),
				Verified:        track.Verified,
				Rejected:        track.Rejected,
				NeedsAttendance: track.Verified && !track.Committed,
				LivenessStatus:  status,
			}
			continue
		}
		if track == nil {
			session.NextID++
			track = &recognitionTrack{
				ID:        fmt.Sprintf("%s-%d", sessionID, session.NextID),
				FirstSeen: now,
				Scores:    make([]float64, 0, livenessMaxFramesForMode(observation.LowLightMode)),
			}
			session.Tracks[observation.Student.ID] = track
		}
		track.LowLightMode = observation.LowLightMode
		maxFrames := livenessMaxFramesForMode(track.LowLightMode)
		decisionTimeout := livenessDecisionTimeoutForMode(track.LowLightMode)
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

func livenessObservationQualityUsable(observation recognitionObservation) bool {
	exposureThreshold := livenessMinExposureScore
	sharpnessThreshold := livenessMinSharpnessScore
	if observation.LowLightMode {
		exposureThreshold = livenessLowLightMinExposureScore
		sharpnessThreshold = livenessLowLightMinSharpnessScore
	}
	return observation.Exposure >= exposureThreshold &&
		observation.Sharpness >= sharpnessThreshold
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
