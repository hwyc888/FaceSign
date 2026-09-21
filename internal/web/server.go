package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hwyc888/FaceSign/internal/face"
	"github.com/hwyc888/FaceSign/internal/store"
)

type Server struct {
	logger             *slog.Logger
	store              *store.Store
	engine             *face.Engine
	matchThreshold     float64
	detectionThreshold float64
	static             http.Handler
}

func New(logger *slog.Logger, st *store.Store, engine *face.Engine, matchThreshold, detectionThreshold float64) (*Server, error) {
	sub, err := fs.Sub(assets, "assets")
	if err != nil {
		return nil, err
	}
	return &Server{
		logger:             logger,
		store:              st,
		engine:             engine,
		matchThreshold:     matchThreshold,
		detectionThreshold: detectionThreshold,
		static:             http.FileServer(http.FS(sub)),
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.health)
	mux.HandleFunc("/api/students", s.students)
	mux.HandleFunc("/api/students/", s.studentAction)
	mux.HandleFunc("/api/recognize", s.recognize)
	mux.HandleFunc("/api/attendance", s.attendance)
	mux.HandleFunc("/", s.staticPage)
	return s.logging(mux)
}

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		s.logger.Debug("http request", "method", r.Method, "path", r.URL.Path, "elapsed", time.Since(started))
	})
}

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
	})
}

func (s *Server) students(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := s.store.ListStudents(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, items)
	case http.MethodPost:
		var in struct {
			StudentNo string `json:"student_no"`
			Name      string `json:"name"`
			ClassName string `json:"class_name"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		student, err := s.store.CreateStudent(r.Context(), in.StudentNo, in.Name, in.ClassName)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusCreated, student)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) studentAction(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/students/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, errors.New("student not found"))
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, errors.New("invalid student id"))
		return
	}
	if len(parts) == 1 && r.Method == http.MethodDelete {
		if err := s.store.DeleteStudent(r.Context(), id); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	if len(parts) == 2 && parts[1] == "face" && r.Method == http.MethodPost {
		s.enrollFace(w, r, id)
		return
	}
	methodNotAllowed(w)
}

func (s *Server) enrollFace(w http.ResponseWriter, r *http.Request, studentID int64) {
	img, err := readImage(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	feature, err := s.engine.Extract(img, s.detectionThreshold)
	if err != nil {
		writeFaceError(w, err)
		return
	}
	if err := s.store.SetFaceSample(r.Context(), studentID, face.Encode(feature)); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) recognize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	img, err := readImage(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	feature, err := s.engine.Extract(img, s.detectionThreshold)
	if err != nil {
		writeFaceError(w, err)
		return
	}
	samples, err := s.store.ListFaceSamples(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	var best store.Student
	bestScore := 0.0
	for _, sample := range samples {
		stored, err := face.Decode(sample.Embedding)
		if err != nil {
			s.logger.Warn("skip invalid face sample", "student_id", sample.Student.ID, "error", err)
			continue
		}
		score := face.Similarity(feature, stored)
		if score > bestScore {
			bestScore = score
			best = sample.Student
		}
	}
	if best.ID == 0 || bestScore < s.matchThreshold {
		writeJSON(w, http.StatusOK, map[string]any{
			"recognized": false,
			"similarity": bestScore,
			"threshold":  s.matchThreshold,
		})
		return
	}
	record, created, err := s.store.MarkAttendance(r.Context(), best, bestScore, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"recognized":          true,
		"student":             best,
		"similarity":          bestScore,
		"attendance":          record,
		"first_checkin_today": created,
	})
}

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

func (s *Server) staticPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/index.html"
		s.static.ServeHTTP(w, r2)
		return
	}
	s.static.ServeHTTP(w, r)
}

func readImage(w http.ResponseWriter, r *http.Request) (image.Image, error) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		return nil, fmt.Errorf("invalid multipart body: %w", err)
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		return nil, errors.New("image file is required")
	}
	defer file.Close()
	return decodeImage(file)
}

func decodeImage(file multipart.File) (image.Image, error) {
	img, _, err := image.Decode(io.LimitReader(file, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	return img, nil
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid json: %w", err)
	}
	return nil
}

func writeFaceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, face.ErrNoFace):
		writeError(w, http.StatusUnprocessableEntity, errors.New("no face detected"))
	case errors.Is(err, face.ErrMultipleFaces):
		writeError(w, http.StatusUnprocessableEntity, errors.New("multiple faces detected; keep only one person in the frame"))
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
}
