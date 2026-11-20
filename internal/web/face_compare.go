package web

import (
	"net/http"
	"sort"

	"github.com/hwyc888/FaceSign/internal/face"
	"github.com/hwyc888/FaceSign/internal/store"
)

type faceCompareCandidate struct {
	Student    store.Student `json:"student"`
	Similarity float64       `json:"similarity"`
	SampleCount int          `json:"sample_count"`
	AboveThreshold bool      `json:"above_threshold"`
}

func (s *Server) faceCompare(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	img, err := readImage(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	feature, err := s.engine.ExtractEnrollment(img, s.currentRecognitionThresholds().DetectionThreshold)
	if err != nil {
		writeFaceError(w, err)
		return
	}
	samples, err := s.cachedFaceSamples(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	threshold := s.currentRecognitionThresholds().MatchThreshold
	byStudent := make(map[int64]*faceCompareCandidate)
	for _, sample := range samples {
		candidate := byStudent[sample.Student.ID]
		if candidate == nil {
			candidate = &faceCompareCandidate{Student: sample.Student}
			byStudent[sample.Student.ID] = candidate
		}
		candidate.SampleCount++
		score := face.Similarity(feature, sample.Feature)
		if score > candidate.Similarity {
			candidate.Similarity = score
		}
	}
	ranked := make([]faceCompareCandidate, 0, len(byStudent))
	for _, candidate := range byStudent {
		candidate.AboveThreshold = candidate.Similarity >= threshold
		ranked = append(ranked, *candidate)
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Similarity == ranked[j].Similarity {
			return ranked[i].Student.StudentNo < ranked[j].Student.StudentNo
		}
		return ranked[i].Similarity > ranked[j].Similarity
	})
	if len(ranked) > 5 {
		ranked = ranked[:5]
	}
	var best *faceCompareCandidate
	if len(ranked) > 0 {
		best = &ranked[0]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"engine": "SCRFD-10GF + ArcFace ResNet50",
		"model_version": s.engine.ModelVersion(),
		"threshold": threshold,
		"face_count": 1,
		"library_students": len(byStudent),
		"best": best,
		"matched": best != nil && best.AboveThreshold,
	})
}
