package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestFaceSamplesAppendDifferentAngles(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "face-samples.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	student, err := st.CreateStudent(ctx, "S-ANGLE-001", "Angle Test", "Class A")
	if err != nil {
		t.Fatal(err)
	}
	front, err := st.AddFaceSampleWithVersion(ctx, student.ID, "正面", "arcface-r50-w600k-v1", []byte{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	left, err := st.AddFaceSampleWithVersion(ctx, student.ID, "左侧", "arcface-r50-w600k-v1", []byte{4, 5, 6})
	if err != nil {
		t.Fatal(err)
	}
	if front.ID == left.ID {
		t.Fatal("different angle samples must be stored as separate records")
	}

	samples, err := st.ListStudentFaceSamples(ctx, student.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 2 {
		t.Fatalf("sample count=%d want=2", len(samples))
	}
	labels := map[string]bool{}
	for _, sample := range samples {
		labels[sample.Label] = true
	}
	if !labels["正面"] || !labels["左侧"] {
		t.Fatalf("angle labels were not retained: %#v", labels)
	}
}
