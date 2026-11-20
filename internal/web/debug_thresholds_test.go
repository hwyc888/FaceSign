package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hwyc888/FaceSign/internal/store"
)

func TestRecognitionDebugThresholdsRequireAdminValidateAndReset(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "debug.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := &Server{
		store: st, matchThreshold: .68, detectionThreshold: .50,
		tracker: newRecognitionTracker(), personTracker: newPersonTracker(),
	}
	user := store.User{ID: 2, CanEditSettings: true}
	policy := `{"match_threshold":0.74,"detection_threshold":0.44,"liveness_pass_threshold":0.70,"liveness_frame_threshold":0.58,"liveness_spoof_threshold":0.38}`

	noAdmin := httptest.NewRequest(http.MethodPut, "/api/debug/thresholds", strings.NewReader(policy))
	noAdmin = requestAsUser(noAdmin, user)
	rec := httptest.NewRecorder()
	s.recognitionDebugThresholds(rec, noAdmin)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("settings editor can adjust PAD: status=%d", rec.Code)
	}

	invalid := `{"match_threshold":0.74,"detection_threshold":0.44,"liveness_pass_threshold":0.54,"liveness_frame_threshold":0.58,"liveness_spoof_threshold":0.38}`
	rec = httptest.NewRecorder()
	s.recognitionDebugThresholds(rec, withTestAdmin(httptest.NewRequest(http.MethodPut, "/api/debug/thresholds", strings.NewReader(invalid))))
	if rec.Code != http.StatusBadRequest || s.currentRecognitionThresholds().MatchThreshold != .68 {
		t.Fatalf("invalid input changed thresholds: status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	s.recognitionDebugThresholds(rec, withTestAdmin(httptest.NewRequest(http.MethodPut, "/api/debug/thresholds", strings.NewReader(policy))))
	if rec.Code != http.StatusOK {
		t.Fatalf("save failed status=%d body=%s", rec.Code, rec.Body.String())
	}
	var saved struct {
		Current store.RecognitionThresholds `json:"current"`
		Modified bool `json:"modified"`
		RequiredFrames int `json:"required_frames"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if !saved.Modified || saved.RequiredFrames != 3 ||
		saved.Current.MatchThreshold != .74 || s.tracker.passThreshold != .70 {
		t.Fatalf("updated values did not take effect: %+v", saved)
	}
	persisted, err := st.LoadRecognitionThresholds(context.Background(), s.baselineThresholds())
	if err != nil || persisted.MatchThreshold != .74 {
		t.Fatalf("updated values not persisted: %+v err=%v", persisted, err)
	}

	rec = httptest.NewRecorder()
	s.resetRecognitionDebugThresholds(rec, requestAsUser(httptest.NewRequest(http.MethodPost, "/api/debug/thresholds/reset", nil), user))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin reset status=%d", rec.Code)
	}
	rec = httptest.NewRecorder()
	s.resetRecognitionDebugThresholds(rec, withTestAdmin(httptest.NewRequest(http.MethodPost, "/api/debug/thresholds/reset", nil)))
	if rec.Code != http.StatusOK {
		t.Fatalf("reset failed: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if current := s.currentRecognitionThresholds(); current != s.baselineThresholds() {
		t.Fatalf("reset did not restore original configuration: %+v", current)
	}
	if s.tracker.passThreshold != livenessPassThreshold {
		t.Fatal("reset did not restore active PAD decisions")
	}
	rec = httptest.NewRecorder()
	s.recognitionDebugThresholds(rec, withTestAdmin(httptest.NewRequest(http.MethodGet, "/api/debug/thresholds", nil)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"modified":false`) {
		t.Fatalf("GET after reset: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRecognitionTrackerTuningResetsPendingVotesAndKeepsThreeFrames(t *testing.T) {
	tracker := newRecognitionTracker()
	student := store.Student{ID: 11, Name: "测试"}
	now := time.Now()
	obs := []recognitionObservation{{Student: student, LiveScore: .59, Similarity: .82}}
	first := tracker.Observe("debug", obs, now)[0]
	if first.Verified || first.Frames != 1 {
		t.Fatalf("baseline should require additional scores: %+v", first)
	}
	tracker.SetThresholds(.55, .58, .40)
	for i := 1; i <= 3; i++ {
		decision := tracker.Observe("debug", obs, now.Add(time.Duration(i)*100*time.Millisecond))[0]
		if decision.Frames != i || (i < 3 && decision.Verified) || (i == 3 && !decision.Verified) {
			t.Fatalf("after threshold change frame %d: %+v", i, decision)
		}
	}
}

func TestRecognitionDebugUIIsAdministratorOnly(t *testing.T) {
	html, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`id="recognitionDebugForm"`, `id="resetRecognitionDebug"`,
		`id="debugMatchThreshold"`, `id="debugLivenessPassThreshold"`,
		`/js/debug.js`, `recognition-debug-card" data-admin-only`,
	} {
		if !strings.Contains(string(html), want) {
			t.Fatalf("debug UI missing %s", want)
		}
	}
}
