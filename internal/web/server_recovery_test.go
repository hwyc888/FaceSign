package web

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoggingRecoversHandlerPanicAsJSON(t *testing.T) {
	s := &Server{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	handler := s.logging(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("camera test panic")
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/cameras/test", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("unexpected content type: %q", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Body.String(), "FaceSign 处理请求时发生内部异常") {
		t.Fatalf("panic should be returned as actionable JSON error: %s", rec.Body.String())
	}
}
