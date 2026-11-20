package web

import (
	"fmt"
	"math"
	"net/http"

	"github.com/hwyc888/FaceSign/internal/store"
)

func baselineRecognitionThresholds(match, detection float64) store.RecognitionThresholds {
	if match <= 0 {
		match = 0.68
	}
	if detection <= 0 {
		detection = 0.50
	}
	return store.RecognitionThresholds{
		MatchThreshold: match,
		DetectionThreshold: detection,
		LivenessPassThreshold: livenessPassThreshold,
		LivenessFrameThreshold: livenessFrameThreshold,
		LivenessSpoofThreshold: livenessSpoofThreshold,
	}
}

func validateRecognitionThresholds(value store.RecognitionThresholds) error {
	limits := []struct {
		label string
		value, min, max float64
	}{
		{"活体通过阈值", value.LivenessPassThreshold, 0.40, 0.95},
		{"单帧有效阈值", value.LivenessFrameThreshold, 0.30, 0.90},
		{"攻击判定阈值", value.LivenessSpoofThreshold, 0.05, 0.50},
	}
	// Match and detection flags accepted any value strictly between 0 and 1
	// before the debug panel existed. Preserve those startup configurations.
	for _, item := range []struct {
		label string
		value float64
	}{
		{"人脸匹配阈值", value.MatchThreshold},
		{"人脸检测阈值", value.DetectionThreshold},
	} {
		if math.IsNaN(item.value) || math.IsInf(item.value, 0) || item.value <= 0 || item.value >= 1 {
			return fmt.Errorf("%s必须在 0 和 1 之间", item.label)
		}
	}
	for _, limit := range limits {
		if math.IsNaN(limit.value) || math.IsInf(limit.value, 0) || limit.value < limit.min || limit.value > limit.max {
			return fmt.Errorf("%s必须介于 %.2f 和 %.2f", limit.label, limit.min, limit.max)
		}
	}
	if !(value.LivenessSpoofThreshold < value.LivenessFrameThreshold &&
		value.LivenessFrameThreshold <= value.LivenessPassThreshold) {
		return fmt.Errorf("活体阈值必须满足：攻击判定 < 单帧有效 <= 活体通过")
	}
	return nil
}

func (s *Server) currentRecognitionThresholds() store.RecognitionThresholds {
	s.debugMu.RLock()
	value := s.debugCurrent
	s.debugMu.RUnlock()
	if value.MatchThreshold == 0 {
		return baselineRecognitionThresholds(s.matchThreshold, s.detectionThreshold)
	}
	return value
}

func (s *Server) baselineThresholds() store.RecognitionThresholds {
	if s.debugDefaults.MatchThreshold != 0 {
		return s.debugDefaults
	}
	return baselineRecognitionThresholds(s.matchThreshold, s.detectionThreshold)
}

func (s *Server) recognitionDebugResponse() map[string]any {
	current := s.currentRecognitionThresholds()
	defaults := s.baselineThresholds()
	return map[string]any{
		"current": current,
		"defaults": defaults,
		"modified": current != defaults,
		"required_frames": livenessMinFrames,
	}
}

func (s *Server) applyRecognitionThresholds(value store.RecognitionThresholds) {
	s.debugCurrent = value
	if s.tracker != nil {
		// Recheck every identity and PAD vote after tuning; never mix old evidence.
		s.tracker.SetThresholds(value.LivenessFrameThreshold, value.LivenessPassThreshold, value.LivenessSpoofThreshold)
		s.tracker.ResetSessions()
	}
	if s.personTracker != nil {
		s.personTracker.ResetSessions()
	}
}

func (s *Server) recognitionDebugThresholds(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.recognitionDebugResponse())
	case http.MethodPut:
		var value store.RecognitionThresholds
		if err := decodeJSON(r, &value); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := validateRecognitionThresholds(value); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		s.debugMu.Lock()
		err := s.store.SaveRecognitionThresholds(r.Context(), value)
		if err == nil {
			s.applyRecognitionThresholds(value)
		}
		s.debugMu.Unlock()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, s.recognitionDebugResponse())
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) resetRecognitionDebugThresholds(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	s.debugMu.Lock()
	err := s.store.ResetRecognitionThresholds(r.Context())
	if err == nil {
		s.applyRecognitionThresholds(s.baselineThresholds())
	}
	s.debugMu.Unlock()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, s.recognitionDebugResponse())
}
