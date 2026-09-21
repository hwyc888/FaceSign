package web

import (
	"net/http"
	"sort"
	"time"

	"github.com/hwyc888/FaceSign/internal/face"
	"github.com/hwyc888/FaceSign/internal/store"
)

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
