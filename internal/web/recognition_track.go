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
	livenessMinFrames         = 4
	livenessMaxFrames         = 6
	livenessFrameThreshold    = 0.60
	livenessFastPassThreshold = 0.80
	livenessPassThreshold     = 0.68
	livenessSpoofThreshold    = 0.40
	livenessDecisionTimeout   = 2 * time.Second
)

type recognitionObservation struct {
	Student    store.Student
	Similarity float64
	LiveScore  float64
}

type recognitionTrack struct {
	ID            string
	StudentID     int64
	FirstSeen     time.Time
	LastSeen      time.Time
	Frames        int
	Scores        []float64
	SimilaritySum float64
	Verified      bool
	Rejected      bool
	Committed     bool
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
	Tracks map[int64]*recognitionTrack
	NextID uint64
}

type recognitionTracker struct {
	mu       sync.Mutex
	sessions map[string]*recognitionTrackSession
}

func newRecognitionTracker() *recognitionTracker {
	return &recognitionTracker{sessions: make(map[string]*recognitionTrackSession)}
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
		if track == nil {
			session.NextID++
			track = &recognitionTrack{
				ID:        fmt.Sprintf("%s-%d", sessionID, session.NextID),
				StudentID: observation.Student.ID,
				FirstSeen: now,
				Scores:    make([]float64, 0, livenessMaxFrames),
			}
			session.Tracks[observation.Student.ID] = track
		}
		track.LastSeen = now
		track.Frames++
		track.SimilaritySum += observation.Similarity
		track.Scores = append(track.Scores, observation.LiveScore)
		if len(track.Scores) > livenessMaxFrames {
			track.Scores = append([]float64(nil), track.Scores[len(track.Scores)-livenessMaxFrames:]...)
		}

		average := averageScores(track.Scores)
		liveVotes := 0
		spoofVotes := 0
		for _, score := range track.Scores {
			if score >= livenessFrameThreshold {
				liveVotes++
			}
			if score < livenessSpoofThreshold {
				spoofVotes++
			}
		}

		timedOut := false
		if !track.Verified && !track.Rejected {
			switch {
			case len(track.Scores) >= 3 && average < livenessSpoofThreshold && spoofVotes >= 2:
				track.Rejected = true
			case len(track.Scores) >= livenessFastFrames &&
				average >= livenessFastPassThreshold &&
				liveVotes == len(track.Scores):
				track.Verified = true
			case len(track.Scores) >= livenessMinFrames &&
				average >= livenessPassThreshold &&
				liveVotes >= livenessMinFrames-1:
				track.Verified = true
			case now.Sub(track.FirstSeen) >= livenessDecisionTimeout || len(track.Scores) >= livenessMaxFrames:
				timedOut = true
			}
		}

		requiredFrames := livenessFastFrames
		if !track.Verified && !track.Rejected && !timedOut && len(track.Scores) >= livenessFastFrames {
			requiredFrames = livenessMinFrames
		}
		if !track.Verified && !track.Rejected && !timedOut && len(track.Scores) >= livenessMinFrames {
			requiredFrames = livenessMaxFrames
		}

		status := "活体验证中"
		switch {
		case track.Verified:
			status = "活体通过"
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
