package web

func (s *Server) tryBeginCameraRecognition(cameraID int64) bool {
	s.cameraRecognitionMu.Lock()
	defer s.cameraRecognitionMu.Unlock()
	if s.cameraRecognitionBusy == nil {
		s.cameraRecognitionBusy = make(map[int64]bool)
	}
	if s.cameraRecognitionBusy[cameraID] {
		return false
	}
	s.cameraRecognitionBusy[cameraID] = true
	return true
}

func (s *Server) endCameraRecognition(cameraID int64) {
	s.cameraRecognitionMu.Lock()
	delete(s.cameraRecognitionBusy, cameraID)
	s.cameraRecognitionMu.Unlock()
}
