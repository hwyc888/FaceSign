package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/hwyc888/FaceSign/internal/store"
)

func TestRecognitionStatsAPIGetAndClear(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "recognition-stats-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if _, err := st.RecordRecognitionStats(context.Background(), []int64{1, 2}, []string{"s-P1"}); err != nil {
		t.Fatal(err)
	}
	s := &Server{store: st}

	getReq := httptest.NewRequest(http.MethodGet, "/api/recognition-stats", nil)
	getRec := httptest.NewRecorder()
	s.recognitionStats(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", getRec.Code, getRec.Body.String())
	}
	var stats store.RecognitionStats
	if err := json.NewDecoder(getRec.Body).Decode(&stats); err != nil {
		t.Fatal(err)
	}
	if stats.Verified != 2 || stats.Unregistered != 1 {
		t.Fatalf("GET stats=%#v", stats)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/recognition-stats", nil)
	deleteRec := httptest.NewRecorder()
	s.recognitionStats(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("DELETE status=%d body=%s", deleteRec.Code, deleteRec.Body.String())
	}

	stats, err = st.RecognitionStats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Verified != 0 || stats.Unregistered != 0 {
		t.Fatalf("DELETE did not clear stats: %#v", stats)
	}
}
