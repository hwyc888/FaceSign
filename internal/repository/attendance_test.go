package repository_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/hwyc888/FaceSign/internal/database"
	"github.com/hwyc888/FaceSign/internal/domain"
	"github.com/hwyc888/FaceSign/internal/repository"
)

func TestRecordRecognitionKeepsEarliestCheckinAndManualPriority(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(filepath.Join(t.TempDir(), "facesign.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	repo := repository.New(db)
	teacher, err := repo.CreateUser(ctx, "teacher", "test-hash", "Teacher", domain.RoleTeacher)
	if err != nil {
		t.Fatalf("create teacher: %v", err)
	}
	class, err := repo.CreateClass(ctx, "Class 1")
	if err != nil {
		t.Fatalf("create class: %v", err)
	}
	student, err := repo.CreateStudent(ctx, "S001", "Student", class.ID)
	if err != nil {
		t.Fatalf("create student: %v", err)
	}
	course, err := repo.CreateCourse(ctx, "Go", teacher.ID)
	if err != nil {
		t.Fatalf("create course: %v", err)
	}
	if err := repo.AddStudentToCourse(ctx, course.ID, student.ID); err != nil {
		t.Fatalf("enroll student: %v", err)
	}

	schedule, err := repo.CreateSchedule(ctx, domain.Schedule{
		CourseID:             course.ID,
		Classroom:            "Room 101",
		Weekday:              1,
		StartMinute:          8 * 60,
		EndMinute:            9 * 60,
		GraceMinutes:         5,
		CheckinBeforeMinutes: 20,
		Enabled:              true,
	})
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}

	day := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	session, _, err := repo.EnsureAttendanceSession(ctx, schedule, "2026-09-14", day.Add(8*time.Hour), day.Add(9*time.Hour))
	if err != nil {
		t.Fatalf("create attendance session: %v", err)
	}

	first := day.Add(8*time.Hour + 2*time.Minute)
	record, err := repo.RecordRecognition(ctx, session.ID, student.ID, domain.AttendanceOnTime, first, 0.81)
	if err != nil {
		t.Fatalf("first recognition: %v", err)
	}
	if record.Status != domain.AttendanceOnTime || record.RecognizedAt != first.Unix() {
		t.Fatalf("first recognition = status %q at %d", record.Status, record.RecognizedAt)
	}

	later := day.Add(8*time.Hour + 10*time.Minute)
	record, err = repo.RecordRecognition(ctx, session.ID, student.ID, domain.AttendanceLate, later, 0.94)
	if err != nil {
		t.Fatalf("second recognition: %v", err)
	}
	if record.Status != domain.AttendanceOnTime {
		t.Fatalf("repeat scan changed status to %q, want %q", record.Status, domain.AttendanceOnTime)
	}
	if record.RecognizedAt != first.Unix() {
		t.Fatalf("repeat scan changed recognized_at to %d, want %d", record.RecognizedAt, first.Unix())
	}
	if record.Similarity != 0.94 {
		t.Fatalf("similarity = %v, want best similarity 0.94", record.Similarity)
	}

	manual, err := repo.RecordManual(ctx, session.ID, student.ID, domain.AttendanceAbsent, "teacher override", later)
	if err != nil {
		t.Fatalf("manual override: %v", err)
	}
	if manual.Source != "manual" || manual.Status != domain.AttendanceAbsent {
		t.Fatalf("manual override not stored: source=%q status=%q", manual.Source, manual.Status)
	}

	afterManual, err := repo.RecordRecognition(ctx, session.ID, student.ID, domain.AttendanceLate, later.Add(time.Minute), 0.99)
	if err != nil {
		t.Fatalf("recognition after manual override: %v", err)
	}
	if afterManual.Source != "manual" || afterManual.Status != domain.AttendanceAbsent || afterManual.Note != "teacher override" {
		t.Fatalf("face scan overwrote manual decision: source=%q status=%q note=%q", afterManual.Source, afterManual.Status, afterManual.Note)
	}
}
