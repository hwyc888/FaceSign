package web

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) classes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := s.store.ListClasses(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, items)
	case http.MethodPost:
		var in struct {
			Name        string `json:"name"`
			SeatRows    int    `json:"seat_rows"`
			SeatsPerRow int    `json:"seats_per_row"`
			LateAfter   string `json:"late_after"`
			Deadline    string `json:"deadline"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		item, err := s.store.CreateClassWithSettings(
			r.Context(), in.Name, in.SeatRows, in.SeatsPerRow, in.LateAfter, in.Deadline,
		)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusCreated, item)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) classAction(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/classes/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, errors.New("class not found"))
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, errors.New("invalid class id"))
		return
	}

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodPut:
			var in struct {
				Name string `json:"name"`
			}
			if err := decodeJSON(r, &in); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			item, err := s.store.RenameClass(r.Context(), id, in.Name)
			if err != nil {
				status := http.StatusBadRequest
				if errors.Is(err, sql.ErrNoRows) {
					status = http.StatusNotFound
				}
				writeError(w, status, err)
				return
			}
			writeJSON(w, http.StatusOK, item)
		case http.MethodDelete:
			if err := s.store.DeleteClass(r.Context(), id); err != nil {
				status := http.StatusBadRequest
				if errors.Is(err, sql.ErrNoRows) {
					status = http.StatusNotFound
				}
				writeError(w, status, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		default:
			methodNotAllowed(w)
		}
		return
	}

	if len(parts) == 2 && parts[1] == "move" && r.Method == http.MethodPost {
		var in struct {
			Direction string `json:"direction"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := s.store.MoveClass(r.Context(), id, in.Direction); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	if len(parts) == 2 && parts[1] == "layout" && r.Method == http.MethodPut {
		var in struct {
			SeatRows    int `json:"seat_rows"`
			SeatsPerRow int `json:"seats_per_row"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		item, err := s.store.UpdateClassLayout(r.Context(), id, in.SeatRows, in.SeatsPerRow)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}

	if len(parts) == 2 && parts[1] == "settings" && r.Method == http.MethodPut {
		var in struct {
			Name        string `json:"name"`
			SeatRows    int    `json:"seat_rows"`
			SeatsPerRow int    `json:"seats_per_row"`
			LateAfter   string `json:"late_after"`
			Deadline    string `json:"deadline"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		item, err := s.store.UpdateClassSettings(
			r.Context(), id, in.Name, in.SeatRows, in.SeatsPerRow, in.LateAfter, in.Deadline,
		)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}

	if len(parts) == 2 && parts[1] == "arrange" && r.Method == http.MethodPost {
		count, err := s.store.AutoArrangeSeats(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "arranged": count})
		return
	}

	if len(parts) == 2 && parts[1] == "seats" && r.Method == http.MethodPost {
		var in struct {
			StudentID    int64 `json:"student_id"`
			TargetSeatNo int   `json:"target_seat_no"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		result, err := s.store.MoveStudentSeatInClass(r.Context(), id, in.StudentID, in.TargetSeatNo)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}

	methodNotAllowed(w)
}
