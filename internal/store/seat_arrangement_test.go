package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestAutoArrangeSelectedSeatsPreservesUnselectedStudents(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "selected-seats.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	class, err := s.CreateClassWithLayout(ctx, "高一1班", 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	unselectedFirst, err := s.CreateStudent(ctx, "U001", "未选甲", class.Name)
	if err != nil {
		t.Fatal(err)
	}
	unselectedSecond, err := s.CreateStudent(ctx, "U002", "未选乙", class.Name)
	if err != nil {
		t.Fatal(err)
	}
	selectedThird, err := s.CreateStudent(ctx, "S003", "选中丙", class.Name)
	if err != nil {
		t.Fatal(err)
	}
	selectedFirst, err := s.CreateStudent(ctx, "S001", "选中甲", class.Name)
	if err != nil {
		t.Fatal(err)
	}
	selectedSecond, err := s.CreateStudent(ctx, "S002", "选中乙", class.Name)
	if err != nil {
		t.Fatal(err)
	}

	for _, assignment := range []struct {
		studentID int64
		seatNo    int
	}{
		{unselectedFirst.ID, 1},
		{unselectedSecond.ID, 2},
		{selectedThird.ID, 5},
		{selectedSecond.ID, 4},
	} {
		if err := s.UpdateStudentSeat(ctx, assignment.studentID, assignment.seatNo); err != nil {
			t.Fatal(err)
		}
	}

	arranged, err := s.AutoArrangeSelectedSeats(ctx, class.ID, []int64{
		selectedThird.ID, selectedFirst.ID, selectedSecond.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if arranged != 3 {
		t.Fatalf("arranged=%d, want 3", arranged)
	}

	assertSeat := func(studentID int64, want int) {
		t.Helper()
		student, err := s.StudentByID(ctx, studentID)
		if err != nil {
			t.Fatal(err)
		}
		if student.SeatNo != want {
			t.Errorf("student %d seat=%d, want %d", studentID, student.SeatNo, want)
		}
	}
	assertSeat(unselectedFirst.ID, 1)
	assertSeat(unselectedSecond.ID, 2)
	assertSeat(selectedFirst.ID, 3)
	assertSeat(selectedSecond.ID, 4)
	assertSeat(selectedThird.ID, 5)
}

func TestAutoArrangeSelectedSeatsCapacityFailureIsAtomic(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "selected-seats-capacity.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	class, err := s.CreateClassWithLayout(ctx, "高一2班", 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	unselectedFirst, err := s.CreateStudent(ctx, "U101", "未选甲", class.Name)
	if err != nil {
		t.Fatal(err)
	}
	unselectedSecond, err := s.CreateStudent(ctx, "U102", "未选乙", class.Name)
	if err != nil {
		t.Fatal(err)
	}
	selectedFirst, err := s.CreateStudent(ctx, "S101", "选中甲", class.Name)
	if err != nil {
		t.Fatal(err)
	}
	selectedSecond, err := s.CreateStudent(ctx, "S102", "选中乙", class.Name)
	if err != nil {
		t.Fatal(err)
	}

	for _, assignment := range []struct {
		studentID int64
		seatNo    int
	}{
		{unselectedFirst.ID, 1},
		{unselectedSecond.ID, 2},
		{selectedFirst.ID, 3},
	} {
		if err := s.UpdateStudentSeat(ctx, assignment.studentID, assignment.seatNo); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := s.AutoArrangeSelectedSeats(ctx, class.ID, []int64{
		selectedFirst.ID, selectedSecond.ID,
	}); err == nil {
		t.Fatal("expected insufficient available seats to fail")
	}

	assertSeat := func(studentID int64, want int) {
		t.Helper()
		student, err := s.StudentByID(ctx, studentID)
		if err != nil {
			t.Fatal(err)
		}
		if student.SeatNo != want {
			t.Errorf("student %d seat=%d after failed arrangement, want %d", studentID, student.SeatNo, want)
		}
	}
	assertSeat(unselectedFirst.ID, 1)
	assertSeat(unselectedSecond.ID, 2)
	assertSeat(selectedFirst.ID, 3)
	assertSeat(selectedSecond.ID, 0)
}
