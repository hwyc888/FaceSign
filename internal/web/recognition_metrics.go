package web

import (
	"sync"
	"time"
)

const recognitionMetricsWindowMinutes = 5

type recognitionMetricBucket struct {
	Detected  int
	Matched   int
	Successful int
}

type recognitionMetricPoint struct {
	Time                string   `json:"time"`
	DetectedFaces       int      `json:"detected_faces"`
	MatchedFaces        int      `json:"matched_faces"`
	SuccessfulFaces     int      `json:"successful_faces"`
	SuccessRatePercent  *float64 `json:"success_rate_percent"`
}

type recognitionMetricsSnapshot struct {
	WindowMinutes      int                      `json:"window_minutes"`
	DetectedFaces      int                      `json:"detected_faces"`
	MatchedFaces       int                      `json:"matched_faces"`
	SuccessfulFaces    int                      `json:"successful_faces"`
	SuccessRatePercent *float64                 `json:"success_rate_percent"`
	Timeline           []recognitionMetricPoint `json:"timeline"`
}

type recognitionMetrics struct {
	mu      sync.Mutex
	buckets map[int64]recognitionMetricBucket
}

func newRecognitionMetrics() *recognitionMetrics {
	return &recognitionMetrics{buckets: make(map[int64]recognitionMetricBucket)}
}

func (m *recognitionMetrics) record(now time.Time, detected, matched, successful int) {
	if m == nil || detected <= 0 {
		return
	}
	if matched < 0 {
		matched = 0
	}
	if matched > detected {
		matched = detected
	}
	if successful < 0 {
		successful = 0
	}
	if successful > matched {
		successful = matched
	}
	minute := now.Unix() / 60
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.buckets == nil {
		m.buckets = make(map[int64]recognitionMetricBucket)
	}
	bucket := m.buckets[minute]
	bucket.Detected += detected
	bucket.Matched += matched
	bucket.Successful += successful
	m.buckets[minute] = bucket
	for key := range m.buckets {
		if key < minute-recognitionMetricsWindowMinutes+1 {
			delete(m.buckets, key)
		}
	}
}

func (m *recognitionMetrics) snapshot(now time.Time) recognitionMetricsSnapshot {
	result := recognitionMetricsSnapshot{
		WindowMinutes: recognitionMetricsWindowMinutes,
		Timeline:      make([]recognitionMetricPoint, 0, recognitionMetricsWindowMinutes),
	}
	if m == nil {
		return result
	}
	currentMinute := now.Unix() / 60
	m.mu.Lock()
	defer m.mu.Unlock()
	for key := range m.buckets {
		if key < currentMinute-recognitionMetricsWindowMinutes+1 {
			delete(m.buckets, key)
		}
	}
	for offset := recognitionMetricsWindowMinutes - 1; offset >= 0; offset-- {
		minute := currentMinute - int64(offset)
		bucket := m.buckets[minute]
		point := recognitionMetricPoint{
			Time:          time.Unix(minute*60, 0).In(now.Location()).Format("15:04"),
			DetectedFaces:    bucket.Detected,
			MatchedFaces:     bucket.Matched,
			SuccessfulFaces:  bucket.Successful,
		}
		if bucket.Detected > 0 {
			rate := float64(bucket.Successful) * 100 / float64(bucket.Detected)
			point.SuccessRatePercent = &rate
		}
		result.Timeline = append(result.Timeline, point)
		result.DetectedFaces += bucket.Detected
		result.MatchedFaces += bucket.Matched
		result.SuccessfulFaces += bucket.Successful
	}
	if result.DetectedFaces > 0 {
		rate := float64(result.SuccessfulFaces) * 100 / float64(result.DetectedFaces)
		result.SuccessRatePercent = &rate
	}
	return result
}

func (s *Server) recordRecognitionFrame(now time.Time, detected, matched, successful int) {
	s.metricsMu.Lock()
	if s.recognitionMetrics == nil {
		s.recognitionMetrics = newRecognitionMetrics()
	}
	metrics := s.recognitionMetrics
	s.metricsMu.Unlock()
	metrics.record(now, detected, matched, successful)
}

func (s *Server) recognitionMetricsSnapshot(now time.Time) recognitionMetricsSnapshot {
	s.metricsMu.Lock()
	if s.recognitionMetrics == nil {
		s.recognitionMetrics = newRecognitionMetrics()
	}
	metrics := s.recognitionMetrics
	s.metricsMu.Unlock()
	return metrics.snapshot(now)
}
