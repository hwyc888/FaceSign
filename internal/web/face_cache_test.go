package web

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/hwyc888/FaceSign/internal/face"
	"github.com/hwyc888/FaceSign/internal/store"
)

func TestFaceFeatureCacheLoadsOnceAndRefreshesAfterMutation(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "face-cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.CreateClass(ctx, "一班"); err != nil {
		t.Fatal(err)
	}
	student, _, err := st.CreateStudentWithFace(ctx, "001", "测试学生", "一班", "正面", face.Encode([]float32{0.1, 0.2, 0.3}))
	if err != nil {
		t.Fatal(err)
	}

	s := &Server{store: st}
	if err := s.reloadFaceCache(ctx); err != nil {
		t.Fatal(err)
	}
	items, err := s.cachedFaceSamples(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Student.ID != student.ID {
		t.Fatalf("unexpected initial cache: %#v", items)
	}

	if _, err := st.AddFaceSample(ctx, student.ID, "侧面", face.Encode([]float32{0.4, 0.5, 0.6})); err != nil {
		t.Fatal(err)
	}
	items, err = s.cachedFaceSamples(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("cache should stay stable until explicit refresh, got %d", len(items))
	}

	s.refreshFaceCacheAfterMutation(ctx)
	items, err = s.cachedFaceSamples(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("cache refresh did not include new sample, got %d", len(items))
	}
}
