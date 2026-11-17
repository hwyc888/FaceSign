package store

import (
	"context"
	"testing"
)

func TestActivateFaceModelArchivesOldVectorsAndPreservesStudent(t *testing.T) {
	ctx := context.Background()
	st, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	student, _, err := st.CreateStudentWithFaceVersion(ctx, "S001", "测试学生", "一班", "正面", "sface-v1", []byte{1, 2, 3, 4})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ActivateFaceModel(ctx, "arcface-r50-w600k-v1"); err != nil {
		t.Fatal(err)
	}
	active, err := st.ListFaceSamples(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Fatalf("legacy embeddings must not remain active, got %d", len(active))
	}
	var archived int
	if err := st.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM face_samples_archive WHERE student_id=?", student.ID).Scan(&archived); err != nil {
		t.Fatal(err)
	}
	if archived != 1 {
		t.Fatalf("expected one archived legacy vector, got %d", archived)
	}
	preserved, err := st.StudentByID(ctx, student.ID)
	if err != nil {
		t.Fatal(err)
	}
	if preserved.Name != "测试学生" || preserved.ClassName != "一班" || preserved.HasFace {
		t.Fatalf("student profile should remain while old face is inactive: %+v", preserved)
	}

	if _, _, err := st.CreateStudentWithFaceVersion(ctx, "S001", "测试学生", "一班", "重新采集", "arcface-r50-w600k-v1", []byte{5, 6, 7, 8}); err == nil {
		t.Fatal("duplicate student number should be rejected")
	}
	if _, err := st.AddFaceSampleWithVersion(ctx, student.ID, "新模型正面", "arcface-r50-w600k-v1", []byte{5, 6, 7, 8}); err != nil {
		t.Fatal(err)
	}
	active, err = st.ListFaceSamples(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].ModelVersion != "arcface-r50-w600k-v1" {
		t.Fatalf("expected one active ArcFace vector, got %+v", active)
	}
}
