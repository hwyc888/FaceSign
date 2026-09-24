package web

import (
	"context"

	"github.com/hwyc888/FaceSign/internal/face"
	"github.com/hwyc888/FaceSign/internal/store"
)

type decodedFaceSample struct {
	SampleID int64
	Student  store.Student
	Feature  []float32
}

func (s *Server) reloadFaceCache(ctx context.Context) error {
	samples, err := s.store.ListFaceSamples(ctx)
	if err != nil {
		return err
	}
	decoded := make([]decodedFaceSample, 0, len(samples))
	for _, sample := range samples {
		feature, err := face.Decode(sample.Embedding)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("skip invalid face sample while building cache",
					"student_id", sample.Student.ID,
					"sample_id", sample.ID,
					"error", err,
				)
			}
			continue
		}
		decoded = append(decoded, decodedFaceSample{
			SampleID: sample.ID,
			Student:  sample.Student,
			Feature:  feature,
		})
	}

	s.faceCacheMu.Lock()
	s.faceCache = decoded
	s.faceCacheLoaded = true
	s.faceCacheMu.Unlock()
	return nil
}

func (s *Server) cachedFaceSamples(ctx context.Context) ([]decodedFaceSample, error) {
	s.faceCacheMu.RLock()
	if s.faceCacheLoaded {
		out := append([]decodedFaceSample(nil), s.faceCache...)
		s.faceCacheMu.RUnlock()
		return out, nil
	}
	s.faceCacheMu.RUnlock()

	if err := s.reloadFaceCache(ctx); err != nil {
		return nil, err
	}
	s.faceCacheMu.RLock()
	out := append([]decodedFaceSample(nil), s.faceCache...)
	s.faceCacheMu.RUnlock()
	return out, nil
}

func (s *Server) refreshFaceCacheAfterMutation(ctx context.Context) {
	if err := s.reloadFaceCache(ctx); err == nil {
		return
	} else if s.logger != nil {
		s.logger.Warn("refresh face feature cache failed; next recognition will retry", "error", err)
	}
	s.faceCacheMu.Lock()
	s.faceCache = nil
	s.faceCacheLoaded = false
	s.faceCacheMu.Unlock()
}
