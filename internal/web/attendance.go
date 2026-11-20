package web

import (
	"net/http"
	"errors"
	"os"
	"strconv"
	"strings"
)

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


func (s *Server) attendanceSnapshot(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) == 4 && parts[0] == "api" && parts[1] == "attendance" && parts[2] == "snapshots" {
		s.serveAttendanceSnapshotImage(w, r, parts[3])
		return
	}
	if len(parts) == 4 && parts[0] == "api" && parts[1] == "attendance" && parts[3] == "snapshots" {
		attendanceID, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil || attendanceID <= 0 {
			http.NotFound(w, r)
			return
		}
		items, err := s.store.ListAttendanceSnapshots(r.Context(), attendanceID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, items)
		return
	}
	// Backward compatibility for the previous single-photo URL.
	if len(parts) == 4 && parts[0] == "api" && parts[1] == "attendance" && parts[3] == "snapshot" {
		attendanceID, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil || attendanceID <= 0 {
			http.NotFound(w, r)
			return
		}
		items, err := s.store.ListAttendanceSnapshots(r.Context(), attendanceID)
		if err != nil || len(items) == 0 {
			http.NotFound(w, r)
			return
		}
		s.serveAttendanceSnapshotImage(w, r, strconv.FormatInt(items[0].ID, 10))
		return
	}
	http.NotFound(w, r)
}

func (s *Server) serveAttendanceSnapshotImage(w http.ResponseWriter, r *http.Request, rawID string) {
	snapshotID, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || snapshotID <= 0 {
		http.NotFound(w, r)
		return
	}
	path, err := s.store.AttendanceSnapshotPath(r.Context(), snapshotID)
	if errors.Is(err, os.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, path)
}

func (s *Server) attendanceSnapshotCache(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		stats, err := s.store.AttendanceSnapshotCacheStats(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, stats)
	case http.MethodDelete:
		stats, err := s.store.ClearAttendanceSnapshots(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"deleted_count": stats.Count,
			"deleted_bytes": stats.Bytes,
			"remaining_count": 0,
		})
	default:
		methodNotAllowed(w)
	}
}
