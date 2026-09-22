package web

import (
	"net/http"
	"sort"
	"strings"
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
	Matched           bool              `json:"matched"`
	Status            string            `json:"status"`
	Student           *store.Student    `json:"student,omitempty"`
	Similarity        float64           `json:"similarity"`
	Attendance        *store.Attendance `json:"attendance,omitempty"`
	FirstCheckinToday bool              `json:"first_checkin_today"`
	Box               faceBox           `json:"box"`
	TrackID           string            `json:"track_id,omitempty"`
	LivenessScore     float64           `json:"liveness_score,omitempty"`
	LivenessStatus    string            `json:"liveness_status,omitempty"`
	LivenessFrames    int               `json:"liveness_frames,omitempty"`
	RequiredFrames    int               `json:"required_frames,omitempty"`
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

type selectedMatch struct {
	DetectionIndex int
	Student        store.Student
	Similarity     float64
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
	selected := make([]selectedMatch, 0, len(detections))
	for _, candidate := range candidates {
		if usedFaces[candidate.DetectionIndex] {
			continue
		}
		student := decoded[candidate.SampleIndex].Student
		if usedStudents[student.ID] {
			continue
		}
		usedFaces[candidate.DetectionIndex] = true
		usedStudents[student.ID] = true
		selected = append(selected, selectedMatch{
			DetectionIndex: candidate.DetectionIndex,
			Student:        student,
			Similarity:     candidate.Score,
		})
	}

	sessionID := recognitionSessionID(r)
	observations := make([]recognitionObservation, 0, len(selected))
	observationDetectionIndexes := make([]int, 0, len(selected))
	for _, match := range selected {
		result := &results[match.DetectionIndex]
		result.Matched = true
		result.Student = &match.Student
		result.Similarity = match.Similarity
		result.Status = "活体验证中"

		score, err := s.liveness.Score(img, detections[match.DetectionIndex].Rectangle)
		if err != nil {
			result.Status = "活体检测失败"
			result.LivenessStatus = "活体检测失败"
			s.logger.Warn("passive liveness inference failed", "student_id", match.Student.ID, "error", err)
			continue
		}
		result.LivenessScore = score
		observations = append(observations, recognitionObservation{
			Student:    match.Student,
			Similarity: match.Similarity,
			LiveScore:  score,
		})
		observationDetectionIndexes = append(observationDetectionIndexes, match.DetectionIndex)
	}

	decisions := s.tracker.Observe(sessionID, observations, time.Now())
	verifiedCount := 0
	pendingCount := 0
	spoofCount := 0
	matchedCount := 0
	for i, decision := range decisions {
		detectionIndex := observationDetectionIndexes[i]
		result := &results[detectionIndex]
		matchedCount++
		result.TrackID = decision.TrackID
		result.LivenessScore = decision.LiveScore
		result.LivenessStatus = decision.LivenessStatus
		result.LivenessFrames = decision.Frames
		result.RequiredFrames = decision.RequiredFrames

		switch {
		case decision.Rejected:
			result.Status = "疑似照片/屏幕"
			spoofCount++
		case decision.Verified:
			result.Recognized = true
			result.Status = "签到通过"
			verifiedCount++
			if decision.NeedsAttendance {
				record, created, err := s.store.MarkAttendance(r.Context(), *result.Student, result.Similarity, time.Now())
				if err != nil {
					writeError(w, http.StatusInternalServerError, err)
					return
				}
				result.Attendance = &record
				result.FirstCheckinToday = created
				s.tracker.Commit(sessionID, decision.TrackID)
			}
		default:
			result.Status = decision.LivenessStatus
			pendingCount++
		}
	}

	unregisteredCount := 0
	for _, result := range results {
		if !result.Matched {
			unregisteredCount++
		}
	}

	response := map[string]any{
		"faces":               results,
		"detected_count":      len(results),
		"matched_count":       matchedCount,
		"recognized_count":    verifiedCount,
		"verified_count":      verifiedCount,
		"pending_count":       pendingCount,
		"spoof_count":         spoofCount,
		"unregistered_count":  unregisteredCount,
		"threshold":           s.matchThreshold,
		"liveness_threshold":  livenessPassThreshold,
		"liveness_min_frames": livenessMinFrames,
	}

	if len(results) == 1 {
		response["recognized"] = results[0].Recognized
		response["matched"] = results[0].Matched
		response["status"] = results[0].Status
		response["similarity"] = results[0].Similarity
		response["liveness_score"] = results[0].LivenessScore
		response["liveness_status"] = results[0].LivenessStatus
		response["liveness_frames"] = results[0].LivenessFrames
		if results[0].Student != nil {
			response["student"] = results[0].Student
			if results[0].Attendance != nil {
				response["attendance"] = results[0].Attendance
				response["first_checkin_today"] = results[0].FirstCheckinToday
			}
		}
	}
	writeJSON(w, http.StatusOK, response)
}

func recognitionSessionID(r *http.Request) string {
	id := strings.TrimSpace(r.Header.Get("X-FaceSign-Session"))
	if id != "" {
		if len(id) > 80 {
			return id[:80]
		}
		return id
	}
	return r.RemoteAddr
}
