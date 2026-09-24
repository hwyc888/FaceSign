package web

import (
	"errors"
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
			in.RealtimeStatusReason == nil {
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
		if err := s.store.UpdateAppSettings(r.Context(), settings); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	default:
		methodNotAllowed(w)
	}
}
