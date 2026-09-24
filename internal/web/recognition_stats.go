package web

import (
	"net/http"
)

func (s *Server) recognitionStats(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		stats, err := s.store.RecognitionStats(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, stats)
	case http.MethodDelete:
		if err := s.store.ClearRecognitionStats(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]int{
			"verified":     0,
			"unregistered": 0,
		})
	default:
		methodNotAllowed(w)
	}
}
