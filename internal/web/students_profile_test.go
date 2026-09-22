package web

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hwyc888/FaceSign/internal/store"
)

func TestStudentProfileUpdateAPI(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "student-profile-web.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	classA, err := st.CreateClassWithLayout(ctx, "一班", 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateClassWithLayout(ctx, "二班", 2, 3); err != nil {
		t.Fatal(err)
	}
	student, err := st.CreateStudent(ctx, "S001", "原姓名", classA.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateStudentSeat(ctx, student.ID, 1); err != nil {
		t.Fatal(err)
	}

	s := &Server{store: st, logger: slog.Default()}
	body := strings.NewReader(`{"student_no":"S009","name":"新姓名","class_name":"二班"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/students/1", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.studentAction(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Student store.Student `json:"student"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Student.StudentNo != "S009" || response.Student.Name != "新姓名" || response.Student.ClassName != "二班" || response.Student.SeatNo != 0 {
		t.Fatalf("unexpected response: %#v", response.Student)
	}
}
