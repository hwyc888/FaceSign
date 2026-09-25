package web

import "time"

const cameraRecognitionMinIdle = 100 * time.Millisecond

func (s *Server) tryBeginCameraRecognition(cameraID int64) bool {
	s.cameraRecognitionMu.Lock()
	defer s.cameraRecognitionMu.Unlock()
	if s.cameraRecognitionBusy == nil {
		s.cameraRecognitionBusy = make(map[int64]bool)
	}
	if s.cameraRecognitionNextAt == nil {
		s.cameraRecognitionNextAt = make(map[int64]time.Time)
	}
	if s.cameraRecognitionBusy[cameraID] {
		return false
	}
	now := time.Now()
	if nextAt := s.cameraRecognitionNextAt[cameraID]; !nextAt.IsZero() && now.Before(nextAt) {
		return false
	}
	delete(s.cameraRecognitionNextAt, cameraID)
	s.cameraRecognitionBusy[cameraID] = true
	return true
}

func (s *Server) endCameraRecognition(cameraID int64) {
	s.cameraRecognitionMu.Lock()
	delete(s.cameraRecognitionBusy, cameraID)
	if s.cameraRecognitionNextAt == nil {
		s.cameraRecognitionNextAt = make(map[int64]time.Time)
	}
	s.cameraRecognitionNextAt[cameraID] = time.Now().Add(cameraRecognitionMinIdle)
	s.cameraRecognitionMu.Unlock()
}
