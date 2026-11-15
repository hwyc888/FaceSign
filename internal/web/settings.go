package web

import (
	"errors"
	"math"
	"net/http"
)

type appSettingsRequest struct {
	AutoStartCheckin          *bool `json:"auto_start_checkin"`
	RealtimeStatusEnabled     *bool `json:"realtime_status_enabled"`
	RealtimeStatusMode        *bool `json:"realtime_status_mode"`
	RealtimeStatusVideo       *bool `json:"realtime_status_video"`
	RealtimeStatusDrop        *bool `json:"realtime_status_drop"`
	RealtimeStatusNetwork     *bool `json:"realtime_status_network"`
	RealtimeStatusRecognition *bool `json:"realtime_status_recognition"`
	RealtimeStatusReason      *bool `json:"realtime_status_reason"`
	LowLightRecognitionMode   *bool `json:"low_light_recognition_mode"`
	DisableLivenessDetection  *bool    `json:"disable_liveness_detection"`
	FaceDetectionThreshold    *float64 `json:"face_detection_threshold"`
	LivenessPassThreshold     *float64 `json:"liveness_pass_threshold"`
	LivenessFrameThreshold    *float64 `json:"liveness_frame_threshold"`
	LivenessSpoofThreshold    *float64 `json:"liveness_spoof_threshold"`
}

func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, err := s.store.AppSettings(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	case http.MethodPut:
		if !requireAdmin(w, r) {
			return
		}
		var in appSettingsRequest
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if in.AutoStartCheckin == nil &&
			in.RealtimeStatusEnabled == nil &&
			in.RealtimeStatusMode == nil &&
			in.RealtimeStatusVideo == nil &&
			in.RealtimeStatusDrop == nil &&
			in.RealtimeStatusNetwork == nil &&
			in.RealtimeStatusRecognition == nil &&
			in.RealtimeStatusReason == nil &&
			in.LowLightRecognitionMode == nil &&
			in.DisableLivenessDetection == nil &&
			in.FaceDetectionThreshold == nil &&
			in.LivenessPassThreshold == nil &&
			in.LivenessFrameThreshold == nil &&
			in.LivenessSpoofThreshold == nil {
			writeError(w, http.StatusBadRequest, errors.New("缺少应用设置"))
			return
		}
		settings, err := s.store.AppSettings(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if in.AutoStartCheckin != nil {
			settings.AutoStartCheckin = *in.AutoStartCheckin
		}
		if in.RealtimeStatusEnabled != nil {
			settings.RealtimeStatusEnabled = *in.RealtimeStatusEnabled
		}
		if in.RealtimeStatusMode != nil {
			settings.RealtimeStatusMode = *in.RealtimeStatusMode
		}
		if in.RealtimeStatusVideo != nil {
			settings.RealtimeStatusVideo = *in.RealtimeStatusVideo
		}
		if in.RealtimeStatusDrop != nil {
			settings.RealtimeStatusDrop = *in.RealtimeStatusDrop
		}
		if in.RealtimeStatusNetwork != nil {
			settings.RealtimeStatusNetwork = *in.RealtimeStatusNetwork
		}
		if in.RealtimeStatusRecognition != nil {
			settings.RealtimeStatusRecognition = *in.RealtimeStatusRecognition
		}
		if in.RealtimeStatusReason != nil {
			settings.RealtimeStatusReason = *in.RealtimeStatusReason
		}
		if in.LowLightRecognitionMode != nil {
			settings.LowLightRecognitionMode = *in.LowLightRecognitionMode
		}
		if in.DisableLivenessDetection != nil {
			settings.DisableLivenessDetection = *in.DisableLivenessDetection
		}
		if in.FaceDetectionThreshold != nil {
			settings.FaceDetectionThreshold = *in.FaceDetectionThreshold
		}
		if in.LivenessPassThreshold != nil {
			settings.LivenessPassThreshold = *in.LivenessPassThreshold
		}
		if in.LivenessFrameThreshold != nil {
			settings.LivenessFrameThreshold = *in.LivenessFrameThreshold
		}
		if in.LivenessSpoofThreshold != nil {
			settings.LivenessSpoofThreshold = *in.LivenessSpoofThreshold
		}
		if !validThreshold(settings.FaceDetectionThreshold, 0.25, 0.80) ||
			!validThreshold(settings.LivenessPassThreshold, 0.50, 0.80) ||
			!validThreshold(settings.LivenessFrameThreshold, 0.45, 0.70) ||
			!validThreshold(settings.LivenessSpoofThreshold, 0.20, 0.45) ||
			settings.LivenessSpoofThreshold >= settings.LivenessPassThreshold ||
			settings.LivenessFrameThreshold > settings.LivenessPassThreshold {
			writeError(w, http.StatusBadRequest, errors.New("识别阈值超出允许范围或活体阈值关系不正确"))
			return
		}
		if err := s.store.UpdateAppSettings(r.Context(), settings); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	default:
		methodNotAllowed(w)
	}
}

func validThreshold(value, minimum, maximum float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= minimum && value <= maximum
}
