package web

import (
	"net/http"

	"github.com/hwyc888/FaceSign/internal/face"
)

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	count, err := s.store.CountStudents(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":              true,
		"engine":          "YuNet + SFace",
		"runtime":         face.RuntimeVersion(),
		"device":          "cpu",
		"python":          false,
		"docker":          false,
		"students":        count,
		"match_threshold": s.matchThreshold,
		"version":         s.version,
	})
}

func (s *Server) versionInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": s.version})
}

