package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestStudentAndAttendanceFlow(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	student, err := s.CreateStudent(ctx, "2026001", "Amy", "A1")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetFaceSample(ctx, student.ID, []byte{1, 2, 3, 4}); err != nil {
		t.Fatal(err)
	}
	students, err := s.ListStudents(ctx)
	if err != nil || len(students) != 1 || !students[0].HasFace {
		t.Fatalf("unexpected students: %#v err=%v", students, err)
	}
	now := time.Date(2026, 9, 21, 8, 30, 0, 0, time.Local)
	_, created, err := s.MarkAttendance(ctx, student, 0.88, now)
	if err != nil || !created {
		t.Fatalf("first attendance created=%v err=%v", created, err)
	}
	_, created, err = s.MarkAttendance(ctx, student, 0.91, now.Add(time.Hour))
	if err != nil || created {
		t.Fatalf("second attendance created=%v err=%v", created, err)
	}
}
