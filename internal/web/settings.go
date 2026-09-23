package web

import (
	"errors"
	"net/http"

	"github.com/hwyc888/FaceSign/internal/store"
)

type appSettingsRequest struct {
	AutoStartCheckin *bool `json:"auto_start_checkin"`
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
		if in.AutoStartCheckin == nil {
			writeError(w, http.StatusBadRequest, errors.New("缺少自动签到设置"))
			return
		}
		settings := store.AppSettings{AutoStartCheckin: *in.AutoStartCheckin}
		if err := s.store.UpdateAppSettings(r.Context(), settings); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	default:
		methodNotAllowed(w)
	}
}
