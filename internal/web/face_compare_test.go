package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hwyc888/FaceSign/internal/store"
)

func TestFaceCompareEndpointIsAdminOnly(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/face-compare", nil)
	req = req.WithContext(context.WithValue(req.Context(), authContextKey{}, store.User{ID: 99}))
	rec := httptest.NewRecorder()
	s.faceCompare(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin face compare status=%d body=%s", rec.Code, rec.Body.String())
	}
}
