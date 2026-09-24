package web

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/hwyc888/FaceSign/internal/store"
	"github.com/pion/webrtc/v4"
)

var (
	cameraPreviewHubIdleTimeout    = 15 * time.Second
	cameraPreviewHubPrepareTimeout = 30 * time.Second
	cameraPreviewPacketStreamFn    = streamRTSPH264Packets
)

type cameraPreviewHub struct {
	key     string
	camera  store.Camera
	source  resolvedRTSPSource
	encoder webRTCH264Encoder

	ctx    context.Context
	cancel context.CancelFunc

	mu          sync.Mutex
	subscribers map[uint64]*webrtc.TrackLocalStaticRTP
	nextID      uint64
	running     bool
	idleTimer   *time.Timer
}

func cameraPreviewHubKey(camera store.Camera) string {
	raw := fmt.Sprintf("%d|%s|%s|%s|%d|%d|%d",
		camera.ID,
		camera.StreamURL,
		camera.Username,
		camera.Password,
		camera.Width,
		camera.Height,
		camera.TimeoutMS,
	)
	sum := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", sum[:])
}

func (s *Server) getOrCreateCameraPreviewHub(camera store.Camera, source resolvedRTSPSource, encoder webRTCH264Encoder) *cameraPreviewHub {
	key := cameraPreviewHubKey(camera)

	s.cameraPreviewHubMu.Lock()
	defer s.cameraPreviewHubMu.Unlock()
	if s.cameraPreviewHubs == nil {
		s.cameraPreviewHubs = make(map[string]*cameraPreviewHub)
	}
	if hub := s.cameraPreviewHubs[key]; hub != nil {
		return hub
	}

	ctx, cancel := context.WithCancel(context.Background())
	hub := &cameraPreviewHub{
		key:         key,
		camera:      camera,
		source:      source,
		encoder:     encoder,
		ctx:         ctx,
		cancel:      cancel,
		subscribers: make(map[uint64]*webrtc.TrackLocalStaticRTP),
	}
	s.cameraPreviewHubs[key] = hub
	hub.scheduleIdleCleanup(s, cameraPreviewHubPrepareTimeout)
	return hub
}

func (hub *cameraPreviewHub) configuration() (resolvedRTSPSource, webRTCH264Encoder) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	return hub.source, hub.encoder
}

func (s *Server) subscribeCameraPreviewHub(hub *cameraPreviewHub, track *webrtc.TrackLocalStaticRTP) func() {
	hub.mu.Lock()
	if hub.idleTimer != nil {
		hub.idleTimer.Stop()
		hub.idleTimer = nil
	}
	hub.nextID++
	id := hub.nextID
	hub.subscribers[id] = track
	if !hub.running {
		hub.running = true
		go s.runCameraPreviewHub(hub)
	}
	hub.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			s.unsubscribeCameraPreviewHub(hub, id)
		})
	}
}

func (s *Server) unsubscribeCameraPreviewHub(hub *cameraPreviewHub, id uint64) {
	hub.mu.Lock()
	delete(hub.subscribers, id)
	if len(hub.subscribers) == 0 {
		hub.scheduleIdleCleanupLocked(s, cameraPreviewHubIdleTimeout)
	}
	hub.mu.Unlock()
}

func (hub *cameraPreviewHub) scheduleIdleCleanup(s *Server, delay time.Duration) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	hub.scheduleIdleCleanupLocked(s, delay)
}

func (hub *cameraPreviewHub) scheduleIdleCleanupLocked(s *Server, delay time.Duration) {
	if hub.idleTimer != nil {
		hub.idleTimer.Stop()
	}
	hub.idleTimer = time.AfterFunc(delay, func() {
		s.stopCameraPreviewHubIfIdle(hub)
	})
}

func (s *Server) stopCameraPreviewHubIfIdle(hub *cameraPreviewHub) {
	s.cameraPreviewHubMu.Lock()
	defer s.cameraPreviewHubMu.Unlock()
	if s.cameraPreviewHubs[hub.key] != hub {
		return
	}

	hub.mu.Lock()
	defer hub.mu.Unlock()
	if len(hub.subscribers) != 0 {
		return
	}
	delete(s.cameraPreviewHubs, hub.key)
	if hub.idleTimer != nil {
		hub.idleTimer.Stop()
		hub.idleTimer = nil
	}
	hub.cancel()
}

func (s *Server) runCameraPreviewHub(hub *cameraPreviewHub) {
	defer func() {
		hub.mu.Lock()
		hub.running = false
		hub.mu.Unlock()
	}()

	encoder := hub.encoder
	for {
		err := cameraPreviewPacketStreamFn(hub.ctx, hub.camera, hub.source, encoder, hub.broadcast)
		if hub.ctx.Err() != nil {
			return
		}

		if encoder.QSVZeroCopy {
			logger := s.logger
			if logger == nil {
				logger = slog.Default()
			}
			logger.Warn("Intel QSV zero-copy preview failed; falling back to CPU decode/scale with QSV encode",
				"camera_id", hub.camera.ID,
				"camera", hub.camera.Name,
				"error", err,
			)
			encoder.QSVZeroCopy = false
			encoder.Mode = "WebRTC H.265→H.264硬件编码(Intel，CPU解码回退)"
			hub.mu.Lock()
			hub.encoder = encoder
			hub.mu.Unlock()
			continue
		}

		logger := s.logger
		if logger == nil {
			logger = slog.Default()
		}
		logger.Warn("shared camera preview stream stopped; retrying",
			"camera_id", hub.camera.ID,
			"camera", hub.camera.Name,
			"error", err,
		)
		select {
		case <-hub.ctx.Done():
			return
		case <-time.After(750 * time.Millisecond):
		}
	}
}

func (hub *cameraPreviewHub) broadcast(packet []byte) error {
	hub.mu.Lock()
	tracks := make([]*webrtc.TrackLocalStaticRTP, 0, len(hub.subscribers))
	for _, track := range hub.subscribers {
		tracks = append(tracks, track)
	}
	hub.mu.Unlock()

	for _, track := range tracks {
		_, _ = track.Write(packet)
	}
	return nil
}
