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
)

type recognitionObservation struct {
	Student         store.Student
	Similarity      float64
	LiveScore       float64
	FaceQuality     float64 // diagnostic only; never gates liveness model scores
	Exposure        float64
	Sharpness       float64
	FaceBrightness  float64
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
	LivenessDisabled bool
	SpoofVotes       int
	RecoveryScores   []float64
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
	Scores          []float64
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
				Frames:         len(track.Scores),
				RequiredFrames: livenessMinFrames,
				LiveScore:      medianScore(track.Scores),
				Verified:       track.Verified,
				LivenessStatus: livenessVerifiedStatus(track),
				Scores:         copyScores(track.Scores),
			}
			continue
		}

		if track == nil {
			session.NextID++
			track = &recognitionTrack{
				ID:        fmt.Sprintf("%s-%d", sessionID, session.NextID),
				FirstSeen: now,
				Scores:    make([]float64, 0, livenessMaxFrames),
			}
			session.Tracks[observation.Student.ID] = track
		}
		track.LastSeen = now
		if track.Rejected {
			// A rejected track must not be cleared by one lucky frame. At the same
			// time, one motion-blurred frame should not keep a real person blocked
			// forever. Accept two model-positive recovery frames within a short
			// three-frame window, then start a completely fresh PAD vote.
			if !observation.InferenceFailed {
				track.RecoveryScores = append(track.RecoveryScores, observation.LiveScore)
				if len(track.RecoveryScores) > 3 {
					track.RecoveryScores = append([]float64(nil), track.RecoveryScores[len(track.RecoveryScores)-3:]...)
				}
			} else {
				track.RecoveryScores = nil
			}
			goodRecoveryFrames := 0
			for _, score := range track.RecoveryScores {
				if score >= livenessPassThreshold {
					goodRecoveryFrames++
				}
			}
			if goodRecoveryFrames < 2 || observation.InferenceFailed || observation.LiveScore < livenessPassThreshold {
				out[i] = trackDecision{
					TrackID:        track.ID,
					Frames:         len(track.RecoveryScores),
					RequiredFrames: 2,
					LiveScore:      medianScore(track.RecoveryScores),
					Rejected:       true,
					LivenessStatus: "疑似照片/屏幕，等待真人重新验证",
					Scores:         copyScores(track.RecoveryScores),
				}
				continue
			}
			// The scene may change from a photo to the real student without the
			// student ID or person box changing. Start independent PAD voting and
			// keep the current good frame as frame 1 instead of throwing it away.
			session.NextID++
			track = &recognitionTrack{
				ID:            fmt.Sprintf("%s-%d", sessionID, session.NextID),
				FirstSeen:     now,
				Scores:        make([]float64, 0, livenessMaxFrames),
				RecoveryScores: nil,
			}
			session.Tracks[observation.Student.ID] = track
		}
		track.LastSeen = now
		decisionTimeout := livenessDecisionTimeout
		if len(track.Scores) > 0 {
			// The short deadline is for a track with no model score yet. Once
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
				LiveScore:      medianScore(track.Scores),
				Scores:         copyScores(track.Scores),
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
		maxFrames := livenessMaxFrames
		track.LastSeen = now
		track.Frames++
		track.SimilaritySum += observation.Similarity
		if observation.LiveScore < livenessSpoofThreshold {
			track.SpoofVotes++
		}
		track.Scores = append(track.Scores, observation.LiveScore)
		if len(track.Scores) > maxFrames {
			track.Scores = append([]float64(nil), track.Scores[len(track.Scores)-maxFrames:]...)
		}

		median := medianScore(track.Scores)
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
				median < livenessSpoofThreshold && track.SpoofVotes >= 2:
				track.Rejected = true
			case len(track.Scores) >= livenessMinFrames &&
				median >= livenessPassThreshold &&
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
			LiveScore:       median,
			Similarity:      track.SimilaritySum / maxLivenessFloat(1, float64(track.Frames)),
			Scores:          copyScores(track.Scores),
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

func livenessVerifiedStatus(track *recognitionTrack) string {
	if track.LivenessDisabled {
		return "活体检测已关闭"
	}
	return "活体通过"
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

func medianScore(scores []float64) float64 {
	if len(scores) == 0 {
		return 0
	}
	ordered := append([]float64(nil), scores...)
	for i := 1; i < len(ordered); i++ {
		value := ordered[i]
		j := i - 1
		for ; j >= 0 && ordered[j] > value; j-- {
			ordered[j+1] = ordered[j]
		}
		ordered[j+1] = value
	}
	middle := len(ordered) / 2
	if len(ordered)%2 == 1 {
		return ordered[middle]
	}
	return (ordered[middle-1] + ordered[middle]) / 2
}

func copyScores(scores []float64) []float64 {
	if len(scores) == 0 {
		return nil
	}
	return append([]float64(nil), scores...)
}
