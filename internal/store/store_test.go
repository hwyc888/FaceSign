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


func TestClassManagementAndStudentLinkage(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "classes.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	first, err := s.CreateClass(ctx, "高一1班")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.CreateClass(ctx, "高一2班")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateClass(ctx, "高一1班"); err == nil {
		t.Fatal("expected duplicate class name to fail")
	}

	if err := s.MoveClass(ctx, second.ID, "up"); err != nil {
		t.Fatal(err)
	}
	classes, err := s.ListClasses(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(classes) != 2 || classes[0].ID != second.ID || classes[1].ID != first.ID {
		t.Fatalf("unexpected class order: %#v", classes)
	}

	student, err := s.CreateStudent(ctx, "2026002", "Bob", "高一1班")
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := s.RenameClass(ctx, first.ID, "高一3班")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != "高一3班" || renamed.StudentCount != 1 {
		t.Fatalf("unexpected renamed class: %#v", renamed)
	}

	students, err := s.ListStudents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(students) != 1 || students[0].ID != student.ID || students[0].ClassName != "高一3班" {
		t.Fatalf("class rename did not update student: %#v", students)
	}
	if err := s.DeleteClass(ctx, first.ID); err == nil {
		t.Fatal("expected class with students to be protected from deletion")
	}
	if err := s.DeleteClass(ctx, second.ID); err != nil {
		t.Fatalf("delete unused class: %v", err)
	}
}

func TestExistingStudentClassesAreImported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-classes.db")
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
			student_id INTEGER NOT NULL,
			embedding BLOB NOT NULL,
			label TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			FOREIGN KEY(student_id) REFERENCES students(id) ON DELETE CASCADE
		);
		CREATE TABLE attendance (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			student_id INTEGER NOT NULL,
			day TEXT NOT NULL,
			checked_at INTEGER NOT NULL,
			similarity REAL NOT NULL,
			FOREIGN KEY(student_id) REFERENCES students(id) ON DELETE CASCADE,
			UNIQUE(student_id, day)
		);
		INSERT INTO students(student_no,name,class_name,created_at) VALUES
			('A01','甲','高二1班',1),
			('A02','乙','高二2班',1),
			('A03','丙','高二1班',1);
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

	classes, err := s.ListClasses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(classes) != 2 || classes[0].Name != "高二1班" || classes[0].StudentCount != 2 || classes[1].Name != "高二2班" {
		t.Fatalf("unexpected imported classes: %#v", classes)
	}
}


func TestUpdateStudentClass(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "update-class.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	if _, err := s.CreateClass(ctx, "高三1班"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateClass(ctx, "高三2班"); err != nil {
		t.Fatal(err)
	}
	student, err := s.CreateStudent(ctx, "C001", "测试学生", "高三1班")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateStudentClass(ctx, student.ID, "高三2班"); err != nil {
		t.Fatal(err)
	}
	students, err := s.ListStudents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(students) != 1 || students[0].ClassName != "高三2班" {
		t.Fatalf("student class not updated: %#v", students)
	}
}
