package web

import "net/http"

func (s *Server) attendance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	items, err := s.store.ListAttendance(r.Context(), r.URL.Query().Get("day"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) attendanceSeats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	board, err := s.store.AttendanceSeatBoard(
		r.Context(),
		r.URL.Query().Get("class_name"),
		r.URL.Query().Get("day"),
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, board)
}
