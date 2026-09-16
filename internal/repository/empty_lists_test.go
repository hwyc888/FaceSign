package repository_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/hwyc888/FaceSign/internal/database"
	"github.com/hwyc888/FaceSign/internal/repository"
)

func TestEmptyListRepositoriesReturnNonNilSlices(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(filepath.Join(t.TempDir(), "facesign.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	repo := repository.New(db)

	users, err := repo.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if users == nil {
		t.Fatal("ListUsers returned nil slice; JSON would be null")
	}

	classes, err := repo.ListClasses(ctx)
	if err != nil {
		t.Fatalf("ListClasses: %v", err)
	}
	if classes == nil {
		t.Fatal("ListClasses returned nil slice; JSON would be null")
	}

	students, err := repo.ListStudents(ctx)
	if err != nil {
		t.Fatalf("ListStudents: %v", err)
	}
	if students == nil {
		t.Fatal("ListStudents returned nil slice; JSON would be null")
	}

	courses, err := repo.ListCourses(ctx)
	if err != nil {
		t.Fatalf("ListCourses: %v", err)
	}
	if courses == nil {
		t.Fatal("ListCourses returned nil slice; JSON would be null")
	}

	enrolled, err := repo.CourseStudents(ctx, 999999)
	if err != nil {
		t.Fatalf("CourseStudents: %v", err)
	}
	if enrolled == nil {
		t.Fatal("CourseStudents returned nil slice; JSON would be null")
	}

	schedules, err := repo.ListSchedules(ctx)
	if err != nil {
		t.Fatalf("ListSchedules: %v", err)
	}
	if schedules == nil {
		t.Fatal("ListSchedules returned nil slice; JSON would be null")
	}
}
