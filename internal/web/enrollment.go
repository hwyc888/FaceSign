package web

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/hwyc888/FaceSign/internal/face"
	"github.com/hwyc888/FaceSign/internal/store"
)

type faceComparison struct {
	SameFound  bool
	SameScore  float64
	Other      store.Student
	OtherScore float64
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

	className := strings.TrimSpace(r.FormValue("class_name"))
	classExists, err := s.store.ClassExists(r.Context(), className)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if !classExists {
		writeError(w, http.StatusBadRequest, errors.New("请选择设置中已经建立的班级"))
		return
	}

	student, sample, err := s.store.CreateStudentWithFace(
		r.Context(),
		r.FormValue("student_no"),
		r.FormValue("name"),
		className,
		r.FormValue("label"),
		face.Encode(feature),
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, friendlyStudentError(err))
		return
	}
	s.refreshFaceCacheAfterMutation(r.Context())
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
	s.refreshFaceCacheAfterMutation(r.Context())
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

func (s *Server) compareFeature(ctx context.Context, feature []float32, targetStudentID int64) (faceComparison, error) {
	samples, err := s.cachedFaceSamples(ctx)
	if err != nil {
		return faceComparison{}, err
	}
	var result faceComparison
	for _, sample := range samples {
		score := face.Similarity(feature, sample.Feature)
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

