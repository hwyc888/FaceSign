package web

import (
	"errors"
	"image"
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
	LivenessTrackID   string            `json:"liveness_track_id,omitempty"`
	QualityScore      float64           `json:"quality_score,omitempty"`
	QualityStatus     string            `json:"quality_status,omitempty"`
	BestQuality       float64           `json:"best_quality,omitempty"`
	LivenessScore     float64           `json:"liveness_score,omitempty"`
	LivenessStatus    string            `json:"liveness_status,omitempty"`
	LivenessFrames    int               `json:"liveness_frames,omitempty"`
	RequiredFrames    int               `json:"required_frames,omitempty"`
	LivenessTimedOut  bool              `json:"liveness_timed_out,omitempty"`
}

type recognitionPerson struct {
	TrackID         string         `json:"track_id"`
	Box             faceBox        `json:"box"`
	Score           float64        `json:"score"`
	Status          string         `json:"status"`
	FaceVisible     bool           `json:"face_visible"`
	FaceQuality     float64        `json:"face_quality,omitempty"`
	BestFaceQuality float64        `json:"best_face_quality,omitempty"`
	Student         *store.Student `json:"student,omitempty"`
	Similarity      float64        `json:"similarity,omitempty"`
}

type matchCandidate struct {
	DetectionIndex int
	Student        store.Student
	Score          float64
	FreshFeature   bool
}

type selectedMatch struct {
	DetectionIndex int
	Student        store.Student
	Similarity     float64
	FreshFeature   bool
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
	s.recognizeImage(w, r, img)
}

func (s *Server) recognizeImage(w http.ResponseWriter, r *http.Request, img image.Image) {
	now := time.Now()
	sessionID := recognitionSessionID(r)

	detections, err := s.engine.DetectAll(img, s.detectionThreshold)
	if err != nil {
		if errors.Is(err, face.ErrNoFace) {
			detections = nil
		} else {
			writeFaceError(w, err)
			return
		}
	}

	loadLevel := normalizeRecognitionLoadLevel(r.Header.Get("X-FaceSign-AI-Load"))
	personDetections, err := s.personDetectionsForRecognition(sessionID, img, now, loadLevel)
	if err != nil {
		s.logger.Warn("person detector failed; continuing with face-derived tracks", "error", err)
		personDetections = nil
	}
	personDetections = addFaceFallbackPersons(personDetections, detections, img.Bounds())
	tracks := s.personTracker.Observe(sessionID, personDetections, now)
	personResults := make([]recognitionPerson, len(tracks))
	personIndex := make(map[string]int, len(tracks))
	for i, track := range tracks {
		status := "等待露脸"
		if track.Student != nil {
			status = "已识别，等待再次露脸"
		}
		personResults[i] = recognitionPerson{
			TrackID:         track.TrackID,
			Box:             boxFromRectangle(track.Rectangle),
			Score:           track.Score,
			Status:          status,
			BestFaceQuality: track.BestQuality,
			Student:         track.Student,
			Similarity:      track.Similarity,
		}
		personIndex[track.TrackID] = i
	}

	results := make([]recognitionFace, len(detections))
	qualities := make([]faceQualityResult, len(detections))
	detectionTrackIDs := make([]string, len(detections))
	needsFeature := make([]bool, len(detections))
	featureEvaluated := make([]bool, len(detections))
	candidates := make([]matchCandidate, 0, len(detections)*2)

	for i, detected := range detections {
		quality := scoreFaceQuality(img, detected)
		qualities[i] = quality
		result := recognitionFace{
			Status:        "等待清晰人脸",
			Box:           boxFromRectangle(detected.Rectangle),
			QualityScore:  quality.Score,
			QualityStatus: quality.Status,
		}

		if trackIndex, ok := associateFaceToPerson(detected.Rectangle, tracks); ok {
			track := tracks[trackIndex]
			detectionTrackIDs[i] = track.TrackID
			result.TrackID = track.TrackID
			result.BestQuality = track.BestQuality
			if pIndex, exists := personIndex[track.TrackID]; exists {
				personResults[pIndex].FaceVisible = true
				personResults[pIndex].FaceQuality = quality.Score
				personResults[pIndex].Status = "等待清晰人脸"
			}
		}

		if quality.Score < minRecognitionFaceQuality {
			if student, similarity, best := s.personTracker.Identity(sessionID, result.TrackID); student != nil {
				result.Matched = true
				result.Student = student
				result.Similarity = similarity
				result.BestQuality = best
				if pIndex, ok := personIndex[result.TrackID]; ok {
					personResults[pIndex].Student = student
					personResults[pIndex].Similarity = similarity
					personResults[pIndex].BestFaceQuality = best
				}
			}
			results[i] = result
			continue
		}

		if result.TrackID != "" && !s.personTracker.NeedFeature(sessionID, result.TrackID, quality.Score, now) {
			if student, similarity, best := s.personTracker.Identity(sessionID, result.TrackID); student != nil {
				candidates = append(candidates, matchCandidate{
					DetectionIndex: i,
					Student:        *student,
					Score:          similarity,
				})
				result.BestQuality = best
			} else {
				result.Status = "等待更佳人脸帧"
			}
		} else {
			needsFeature[i] = true
		}
		results[i] = result
	}

	var decoded []decodedFaceSample
	if anyTrue(needsFeature) {
		decoded, err = s.cachedFaceSamples(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		for i, need := range needsFeature {
			if !need {
				continue
			}
			feature, err := s.engine.Feature(img, detections[i])
			if err != nil {
				s.logger.Warn("best-frame feature extraction failed", "track_id", detectionTrackIDs[i], "error", err)
				continue
			}
			featureEvaluated[i] = true
			for _, sample := range decoded {
				score := face.Similarity(feature, sample.Feature)
				if score >= s.matchThreshold {
					candidates = append(candidates, matchCandidate{
						DetectionIndex: i,
						Student:        sample.Student,
						Score:          score,
						FreshFeature:   true,
					})
				}
			}
		}
	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Score > candidates[j].Score })
	usedFaces := make(map[int]bool)
	usedStudents := make(map[int64]bool)
	selected := make([]selectedMatch, 0, len(detections))
	selectedByDetection := make(map[int]selectedMatch)
	for _, candidate := range candidates {
		if usedFaces[candidate.DetectionIndex] || usedStudents[candidate.Student.ID] {
			continue
		}
		usedFaces[candidate.DetectionIndex] = true
		usedStudents[candidate.Student.ID] = true
		match := selectedMatch{
			DetectionIndex: candidate.DetectionIndex,
			Student:        candidate.Student,
			Similarity:     candidate.Score,
			FreshFeature:   candidate.FreshFeature,
		}
		selected = append(selected, match)
		selectedByDetection[candidate.DetectionIndex] = match
	}

	for i, need := range needsFeature {
		if !need {
			continue
		}
		if match, ok := selectedByDetection[i]; ok && match.FreshFeature {
			student := match.Student
			s.personTracker.RecordFeature(sessionID, detectionTrackIDs[i], qualities[i].Score, &student, match.Similarity, now)
			if _, _, best := s.personTracker.Identity(sessionID, detectionTrackIDs[i]); best > 0 {
				results[i].BestQuality = best
			}
		} else {
			s.personTracker.RecordFeature(sessionID, detectionTrackIDs[i], qualities[i].Score, nil, 0, now)
			if featureEvaluated[i] {
				results[i].Status = "未录入"
			}
			if _, _, best := s.personTracker.Identity(sessionID, detectionTrackIDs[i]); best > 0 {
				results[i].BestQuality = best
			}
		}
	}

	observations := make([]recognitionObservation, 0, len(selected))
	observationDetectionIndexes := make([]int, 0, len(selected))
	for _, match := range selected {
		result := &results[match.DetectionIndex]
		result.Matched = true
		student := match.Student
		result.Student = &student
		result.Similarity = match.Similarity
		result.Status = "活体验证中"

		if pIndex, ok := personIndex[result.TrackID]; ok {
			personResults[pIndex].Student = &student
			personResults[pIndex].Similarity = match.Similarity
			personResults[pIndex].BestFaceQuality = result.BestQuality
			personResults[pIndex].Status = "活体验证中"
		}

		score, err := s.liveness.Score(img, detections[match.DetectionIndex].Rectangle)
		if err != nil {
			result.Status = "活体检测失败"
			result.LivenessStatus = "活体检测失败"
			if pIndex, ok := personIndex[result.TrackID]; ok {
				personResults[pIndex].Status = "活体检测失败"
			}
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

	decisions := s.tracker.Observe(sessionID, observations, now)
	verifiedCount := 0
	pendingCount := 0
	timeoutCount := 0
	spoofCount := 0
	matchedCount := 0
	verifiedStudentIDs := make([]int64, 0, len(decisions))
	for i, decision := range decisions {
		detectionIndex := observationDetectionIndexes[i]
		result := &results[detectionIndex]
		matchedCount++
		result.LivenessTrackID = decision.TrackID
		result.LivenessScore = decision.LiveScore
		result.LivenessStatus = decision.LivenessStatus
		result.LivenessFrames = decision.Frames
		result.RequiredFrames = decision.RequiredFrames
		result.LivenessTimedOut = decision.TimedOut

		switch {
		case decision.Rejected:
			result.Status = "疑似照片/屏幕"
			spoofCount++
		case decision.Verified:
			result.Recognized = true
			result.Status = "签到通过"
			verifiedCount++
			if result.Student != nil {
				verifiedStudentIDs = append(verifiedStudentIDs, result.Student.ID)
			}
			if decision.NeedsAttendance {
				record, created, err := s.store.MarkAttendance(r.Context(), *result.Student, result.Similarity, now)
				if err != nil {
					writeError(w, http.StatusInternalServerError, err)
					return
				}
				result.Attendance = &record
				result.FirstCheckinToday = created
				s.tracker.Commit(sessionID, decision.TrackID)
			}
		case decision.TimedOut:
			result.Status = decision.LivenessStatus
			timeoutCount++
		default:
			result.Status = decision.LivenessStatus
			pendingCount++
		}

		if pIndex, ok := personIndex[result.TrackID]; ok {
			personResults[pIndex].Status = result.Status
			personResults[pIndex].Student = result.Student
			personResults[pIndex].Similarity = result.Similarity
			personResults[pIndex].BestFaceQuality = result.BestQuality
		}
	}

	unregisteredCount := 0
	unregisteredTrackIDs := make([]string, 0, len(results))
	for _, result := range results {
		if !result.Matched && result.QualityScore >= minRecognitionFaceQuality {
			unregisteredCount++
			if result.TrackID != "" {
				unregisteredTrackIDs = append(unregisteredTrackIDs, result.TrackID)
			}
		}
	}

	cumulativeStats, statsErr := s.store.RecordRecognitionStats(r.Context(), verifiedStudentIDs, unregisteredTrackIDs)
	if statsErr != nil {
		s.logger.Warn("record recognition stats failed", "error", statsErr)
	}

	waitingFaceCount := 0
	for _, item := range personResults {
		if !item.FaceVisible || item.FaceQuality < minRecognitionFaceQuality {
			waitingFaceCount++
		}
	}

	response := map[string]any{
		"faces":                   results,
		"frame_width":             img.Bounds().Dx(),
		"frame_height":            img.Bounds().Dy(),
		"persons":                 personResults,
		"detected_count":          len(results),
		"tracked_person_count":    len(personResults),
		"waiting_face_count":      waitingFaceCount,
		"matched_count":           matchedCount,
		"recognized_count":        verifiedCount,
		"verified_count":          verifiedCount,
		"pending_count":           pendingCount,
		"timeout_count":           timeoutCount,
		"spoof_count":             spoofCount,
		"unregistered_count":      unregisteredCount,
		"threshold":               s.matchThreshold,
		"face_quality_threshold":  minRecognitionFaceQuality,
		"liveness_threshold":      livenessPassThreshold,
		"liveness_fast_threshold": livenessFastPassThreshold,
		"liveness_fast_frames":    livenessFastFrames,
		"liveness_min_frames":     livenessMinFrames,
		"liveness_max_frames":     livenessMaxFrames,
		"liveness_timeout_ms":     livenessDecisionTimeout.Milliseconds(),
	}
	if statsErr == nil {
		response["verified_total"] = cumulativeStats.Verified
		response["unregistered_total"] = cumulativeStats.Unregistered
	}

	if len(results) == 1 {
		response["recognized"] = results[0].Recognized
		response["matched"] = results[0].Matched
		response["status"] = results[0].Status
		response["similarity"] = results[0].Similarity
		response["quality_score"] = results[0].QualityScore
		response["quality_status"] = results[0].QualityStatus
		response["best_quality"] = results[0].BestQuality
		response["liveness_score"] = results[0].LivenessScore
		response["liveness_status"] = results[0].LivenessStatus
		response["liveness_frames"] = results[0].LivenessFrames
		response["required_frames"] = results[0].RequiredFrames
		response["liveness_timed_out"] = results[0].LivenessTimedOut
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

func boxFromRectangle(rect image.Rectangle) faceBox {
	return faceBox{
		X:      rect.Min.X,
		Y:      rect.Min.Y,
		Width:  rect.Dx(),
		Height: rect.Dy(),
	}
}

func anyTrue(values []bool) bool {
	for _, value := range values {
		if value {
			return true
		}
	}
	return false
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
