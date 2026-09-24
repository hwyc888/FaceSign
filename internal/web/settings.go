package web

import (
	"errors"
	"net/http"

	"github.com/hwyc888/FaceSign/internal/store"
)

type appSettingsRequest struct {
	AutoStartCheckin      *bool `json:"auto_start_checkin"`
	RealtimeStatusEnabled *bool `json:"realtime_status_enabled"`
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
		if in.AutoStartCheckin == nil && in.RealtimeStatusEnabled == nil {
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
		if err := s.store.UpdateAppSettings(r.Context(), settings); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	default:
		methodNotAllowed(w)
	}
}
