package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRecognitionStatsPersistDeduplicateAndClear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recognition-stats.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	stats, err := s.RecognitionStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Verified != 0 || stats.Unregistered != 0 {
		t.Fatalf("unexpected initial stats: %#v", stats)
	}

	stats, err = s.RecordRecognitionStats(ctx,
		[]int64{1, 1, 2},
		[]string{"session-a-P1", "session-a-P1", "session-a-P2"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Verified != 2 || stats.Unregistered != 2 {
		t.Fatalf("deduplicated stats=%#v want verified=2 unregistered=2", stats)
	}

	stats, err = s.RecordRecognitionStats(ctx,
		[]int64{1},
		[]string{"session-a-P2"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Verified != 2 || stats.Unregistered != 2 {
		t.Fatalf("repeat subjects changed stats: %#v", stats)
	}

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	stats, err = s.RecognitionStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Verified != 2 || stats.Unregistered != 2 {
		t.Fatalf("stats did not persist across reopen: %#v", stats)
	}

	if err := s.ClearRecognitionStats(ctx); err != nil {
		t.Fatal(err)
	}
	stats, err = s.RecognitionStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Verified != 0 || stats.Unregistered != 0 {
		t.Fatalf("clear did not reset stats: %#v", stats)
	}
}
