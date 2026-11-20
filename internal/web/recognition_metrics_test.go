package web

import (
	"testing"
	"time"
)

func TestRecognitionMetricsCalculateFiveMinuteEndToEndSuccessRate(t *testing.T) {
	metrics := newRecognitionMetrics()
	now := time.Date(2026, 11, 20, 12, 30, 20, 0, time.Local)
	metrics.record(now.Add(-time.Minute), 3, 3, 2)
	metrics.record(now, 2, 2, 1)
	snapshot := metrics.snapshot(now)
	if snapshot.WindowMinutes != 5 || snapshot.DetectedFaces != 5 || snapshot.MatchedFaces != 5 || snapshot.SuccessfulFaces != 3 {
		t.Fatalf("unexpected recognition totals: %#v", snapshot)
	}
	if snapshot.SuccessRatePercent == nil || *snapshot.SuccessRatePercent != 60 {
		t.Fatalf("success rate=%v, want 60%%", snapshot.SuccessRatePercent)
	}
	if len(snapshot.Timeline) != 5 || snapshot.Timeline[3].DetectedFaces != 3 || snapshot.Timeline[3].SuccessfulFaces != 2 || snapshot.Timeline[4].DetectedFaces != 2 || snapshot.Timeline[4].SuccessfulFaces != 1 {
		t.Fatalf("unexpected five-minute timeline: %#v", snapshot.Timeline)
	}
}

func TestRecognitionMetricsExcludeEmptyAndExpiredBuckets(t *testing.T) {
	metrics := newRecognitionMetrics()
	now := time.Date(2026, 11, 20, 12, 30, 20, 0, time.Local)
	metrics.record(now.Add(-6*time.Minute), 4, 4, 4)
	metrics.record(now, 0, 0, 0)
	snapshot := metrics.snapshot(now)
	if snapshot.DetectedFaces != 0 || snapshot.SuccessRatePercent != nil {
		t.Fatalf("empty window should not report a success rate: %#v", snapshot)
	}
	for _, point := range snapshot.Timeline {
		if point.DetectedFaces != 0 {
			t.Fatalf("expired bucket leaked into timeline: %#v", point)
		}
	}
}
