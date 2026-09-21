package web

import (
	"context"
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
	"sort"
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
	version            string
	home               []byte
	static             http.Handler
}

func New(logger *slog.Logger, st *store.Store, engine *face.Engine, matchThreshold, detectionThreshold float64, version string) (*Server, error) {
	sub, err := fs.Sub(assets, "assets")
	if err != nil {
		return nil, err
	}
	home, err := assets.ReadFile("assets/index.html")
	if err != nil {
		return nil, err
	}
	return &Server{
		logger:             logger,
		store:              st,
		engine:             engine,
		matchThreshold:     matchThreshold,
		detectionThreshold: detectionThreshold,
		version:            version,
		home:               home,
		static:             http.FileServer(http.FS(sub)),
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.health)
	mux.HandleFunc("/api/version", s.versionInfo)
	mux.HandleFunc("/api/students", s.students)
	mux.HandleFunc("/api/students/", s.studentAction)
	mux.HandleFunc("/api/enrollment/check", s.enrollmentCheck)
	mux.HandleFunc("/api/enrollment/create", s.createEnrollment)
	mux.HandleFunc("/api/recognize", s.recognize)
	mux.HandleFunc("/api/attendance", s.attendance)
	mux.HandleFunc("/", s.root)
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

func (s *Server) root(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-FaceSign-Version", s.version)
		s.static.ServeHTTP(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-FaceSign-Version", s.version)
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = w.Write(s.home)
	}
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
			writeError(w, http.StatusBadRequest, friendlyStudentError(err))
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
	studentID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || studentID <= 0 {
		writeError(w, http.StatusBadRequest, errors.New("invalid student id"))
		return
	}

	if len(parts) == 1 && r.Method == http.MethodDelete {
		if err := s.store.DeleteStudent(r.Context(), studentID); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	if len(parts) == 2 && parts[1] == "face" && r.Method == http.MethodPost {
		s.addFaceSample(w, r, studentID)
		return
	}
	if len(parts) == 2 && parts[1] == "faces" {
		switch r.Method {
		case http.MethodGet:
			items, err := s.store.ListStudentFaceSamples(r.Context(), studentID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, http.StatusOK, items)
		case http.MethodPost:
			s.addFaceSample(w, r, studentID)
		default:
			methodNotAllowed(w)
		}
		return
	}
	if len(parts) == 3 && parts[1] == "faces" && r.Method == http.MethodDelete {
		sampleID, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil || sampleID <= 0 {
			writeError(w, http.StatusBadRequest, errors.New("invalid face sample id"))
			return
		}
		if err := s.store.DeleteFaceSample(r.Context(), studentID, sampleID); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	methodNotAllowed(w)
}

func (s *Server) enrollmentCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	feature, err := s.readEnrollmentFeature(w, r)
	if err != nil {
		s.writeEnrollmentError(w, err)
		return
	}
	comparison, err := s.compareFeature(r.Context(), feature, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	duplicate := comparison.Other.ID != 0 && comparison.OtherScore >= s.matchThreshold
	response := map[string]any{
		"duplicate":  duplicate,
		"similarity": comparison.OtherScore,
		"threshold":  s.matchThreshold,
	}
	if duplicate {
		response["student"] = comparison.Other
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) createEnrollment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	feature, err := s.readEnrollmentFeature(w, r)
	if err != nil {
		s.writeEnrollmentError(w, err)
		return
	}
	comparison, err := s.compareFeature(r.Context(), feature, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if comparison.Other.ID != 0 && comparison.OtherScore >= s.matchThreshold {
		writeDuplicate(w, comparison.Other, comparison.OtherScore)
		return
	}

	student, sample, err := s.store.CreateStudentWithFace(
		r.Context(),
		r.FormValue("student_no"),
		r.FormValue("name"),
		r.FormValue("class_name"),
		r.FormValue("label"),
		face.Encode(feature),
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, friendlyStudentError(err))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"ok":      true,
		"student": student,
		"sample":  sample,
	})
}

func (s *Server) addFaceSample(w http.ResponseWriter, r *http.Request, studentID int64) {
	feature, err := s.readEnrollmentFeature(w, r)
	if err != nil {
		s.writeEnrollmentError(w, err)
		return
	}
	comparison, err := s.compareFeature(r.Context(), feature, studentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	if comparison.Other.ID != 0 &&
		comparison.OtherScore >= s.matchThreshold &&
		(!comparison.SameFound || comparison.OtherScore > comparison.SameScore) {
		writeDuplicate(w, comparison.Other, comparison.OtherScore)
		return
	}
	if comparison.SameFound && comparison.SameScore < s.matchThreshold {
		writeError(w, http.StatusUnprocessableEntity, errors.New("该人脸与该学生已有样本不一致，请确认人员后再补充"))
		return
	}
	if comparison.SameFound && comparison.SameScore >= 0.995 {
		writeError(w, http.StatusConflict, errors.New("该角度与已有样本过于相似，请换一个角度再补充"))
		return
	}

	sample, err := s.store.AddFaceSample(r.Context(), studentID, r.FormValue("label"), face.Encode(feature))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"sample":     sample,
		"similarity": comparison.SameScore,
	})
}

func (s *Server) readEnrollmentFeature(w http.ResponseWriter, r *http.Request) ([]float32, error) {
	img, err := readImage(w, r)
	if err != nil {
		return nil, err
	}
	return s.engine.ExtractEnrollment(img, s.detectionThreshold)
}

type faceComparison struct {
	SameFound  bool
	SameScore  float64
	Other      store.Student
	OtherScore float64
}

func (s *Server) compareFeature(ctx context.Context, feature []float32, targetStudentID int64) (faceComparison, error) {
	samples, err := s.store.ListFaceSamples(ctx)
	if err != nil {
		return faceComparison{}, err
	}
	var result faceComparison
	for _, sample := range samples {
		stored, err := face.Decode(sample.Embedding)
		if err != nil {
			s.logger.Warn("skip invalid face sample", "student_id", sample.Student.ID, "sample_id", sample.ID, "error", err)
			continue
		}
		score := face.Similarity(feature, stored)
		if targetStudentID > 0 && sample.Student.ID == targetStudentID {
			result.SameFound = true
			if score > result.SameScore {
				result.SameScore = score
			}
			continue
		}
		if score > result.OtherScore {
			result.OtherScore = score
			result.Other = sample.Student
		}
	}
	return result, nil
}

func (s *Server) writeEnrollmentError(w http.ResponseWriter, err error) {
	if strings.Contains(err.Error(), "image file is required") || strings.Contains(err.Error(), "decode image") || strings.Contains(err.Error(), "multipart") {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeFaceError(w, err)
}

func writeDuplicate(w http.ResponseWriter, student store.Student, similarity float64) {
	writeJSON(w, http.StatusConflict, map[string]any{
		"error":      "该人脸已经录入，不能重复建立学生",
		"duplicate":  true,
		"student":    student,
		"similarity": similarity,
	})
}

func friendlyStudentError(err error) error {
	if strings.Contains(err.Error(), "students.student_no") {
		return errors.New("该学号已经存在，请检查后再录入")
	}
	return err
}

type faceBox struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type recognitionFace struct {
	Recognized        bool              `json:"recognized"`
	Status            string            `json:"status"`
	Student           *store.Student    `json:"student,omitempty"`
	Similarity        float64           `json:"similarity"`
	Attendance        *store.Attendance `json:"attendance,omitempty"`
	FirstCheckinToday bool              `json:"first_checkin_today"`
	Box               faceBox           `json:"box"`
}

type decodedFaceSample struct {
	Student store.Student
	Feature []float32
}

type matchCandidate struct {
	DetectionIndex int
	SampleIndex    int
	Score          float64
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
	detections, err := s.engine.ExtractAll(img, s.detectionThreshold)
	if err != nil {
		writeFaceError(w, err)
		return
	}
	samples, err := s.store.ListFaceSamples(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	decoded := make([]decodedFaceSample, 0, len(samples))
	for _, sample := range samples {
		stored, err := face.Decode(sample.Embedding)
		if err != nil {
			s.logger.Warn("skip invalid face sample", "student_id", sample.Student.ID, "sample_id", sample.ID, "error", err)
			continue
		}
		decoded = append(decoded, decodedFaceSample{Student: sample.Student, Feature: stored})
	}

	results := make([]recognitionFace, len(detections))
	candidates := make([]matchCandidate, 0, len(detections)*len(decoded))
	for i, detected := range detections {
		rect := detected.Rectangle
		results[i] = recognitionFace{
			Status: "未录入",
			Box: faceBox{
				X:      rect.Min.X,
				Y:      rect.Min.Y,
				Width:  rect.Dx(),
				Height: rect.Dy(),
			},
		}
		for j, sample := range decoded {
			score := face.Similarity(detected.Feature, sample.Feature)
			if score > results[i].Similarity {
				results[i].Similarity = score
			}
			if score >= s.matchThreshold {
				candidates = append(candidates, matchCandidate{
					DetectionIndex: i,
					SampleIndex:    j,
					Score:          score,
				})
			}
		}
	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Score > candidates[j].Score })
	usedFaces := make(map[int]bool)
	usedStudents := make(map[int64]bool)
	recognizedCount := 0
	for _, candidate := range candidates {
		if usedFaces[candidate.DetectionIndex] {
			continue
		}
		student := decoded[candidate.SampleIndex].Student
		if usedStudents[student.ID] {
			continue
		}

		record, created, err := s.store.MarkAttendance(r.Context(), student, candidate.Score, time.Now())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		results[candidate.DetectionIndex].Recognized = true
		results[candidate.DetectionIndex].Status = "已录入"
		results[candidate.DetectionIndex].Student = &student
		results[candidate.DetectionIndex].Similarity = candidate.Score
		results[candidate.DetectionIndex].Attendance = &record
		results[candidate.DetectionIndex].FirstCheckinToday = created
		usedFaces[candidate.DetectionIndex] = true
		usedStudents[student.ID] = true
		recognizedCount++
	}

	response := map[string]any{
		"faces":              results,
		"detected_count":     len(results),
		"recognized_count":   recognizedCount,
		"unregistered_count": len(results) - recognizedCount,
		"threshold":          s.matchThreshold,
	}

	if len(results) == 1 {
		response["recognized"] = results[0].Recognized
		response["similarity"] = results[0].Similarity
		if results[0].Student != nil {
			response["student"] = results[0].Student
			response["attendance"] = results[0].Attendance
			response["first_checkin_today"] = results[0].FirstCheckinToday
		}
	}
	writeJSON(w, http.StatusOK, response)
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
		writeError(w, http.StatusUnprocessableEntity, errors.New("未检测到清晰人脸，请正对摄像头并靠近一些"))
	case errors.Is(err, face.ErrMultipleFaces):
		writeError(w, http.StatusUnprocessableEntity, errors.New("录入时只能有一张明显人脸，请确保镜头前只有一人"))
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
