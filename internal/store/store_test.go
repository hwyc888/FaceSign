package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestStudentMultipleFaceSamplesAndAttendanceFlow(t *testing.T) {
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
	if _, err := s.AddFaceSample(ctx, student.ID, "正面", []byte{1, 2, 3, 4}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddFaceSample(ctx, student.ID, "左侧", []byte{5, 6, 7, 8}); err != nil {
		t.Fatal(err)
	}

	students, err := s.ListStudents(ctx)
	if err != nil || len(students) != 1 || !students[0].HasFace || students[0].FaceCount != 2 {
		t.Fatalf("unexpected students: %#v err=%v", students, err)
	}
	samples, err := s.ListStudentFaceSamples(ctx, student.ID)
	if err != nil || len(samples) != 2 {
		t.Fatalf("unexpected samples: %#v err=%v", samples, err)
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

func TestOldSingleFaceSchemaMigratesToMultipleSamples(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE students (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			student_no TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			class_name TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL
		);
		CREATE TABLE face_samples (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			student_id INTEGER NOT NULL UNIQUE,
			embedding BLOB NOT NULL,
			created_at INTEGER NOT NULL,
			FOREIGN KEY(student_id) REFERENCES students(id) ON DELETE CASCADE
		);
		INSERT INTO students(id,student_no,name,class_name,created_at) VALUES(1,'LEGACY001','Legacy','A1',1);
		INSERT INTO face_samples(id,student_id,embedding,created_at) VALUES(1,1,x'01020304',1);
	`)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	if _, err := s.AddFaceSample(ctx, 1, "右侧", []byte{5, 6, 7, 8}); err != nil {
		t.Fatalf("adding second sample after migration: %v", err)
	}
	students, err := s.ListStudents(ctx)
	if err != nil || len(students) != 1 || students[0].FaceCount != 2 {
		t.Fatalf("legacy migration did not preserve/add samples: %#v err=%v", students, err)
	}
}
