package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hwyc888/FaceSign/internal/store"
)

const (
	networkCameraStreamFreshFor = 2 * time.Second
	networkCameraStreamRetryMin  = 500 * time.Millisecond
	networkCameraStreamRetryMax  = 5 * time.Second
	maxContinuousFrameBytes      = 16 << 20
)

type pooledNetworkCameraFrame struct {
	data      []byte
	width     int
	height    int
	sequence  uint64
	source    string
	updatedAt time.Time
}

type networkCameraStream struct {
	key    string
	camera store.Camera
	cancel context.CancelFunc

	mu      sync.RWMutex
	frame   pooledNetworkCameraFrame
	lastErr error
	notify  chan struct{}
}

func newNetworkCameraStream(camera store.Camera, key string) *networkCameraStream {
	return &networkCameraStream{
		key:    key,
		camera: camera,
		notify: make(chan struct{}),
	}
}

func (stream *networkCameraStream) publish(data []byte, source string) error {
	if len(data) == 0 {
		return errors.New("连续流返回空画面")
	}
	if len(data) > maxContinuousFrameBytes {
		return errors.New("连续流单帧超过16MB限制")
	}

	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("连续流JPEG帧无法解析: %w", err)
	}
	if !strings.EqualFold(format, "jpeg") {
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("连续流图像无法解码: %w", err)
		}
		data, config.Width, config.Height, err = encodeJPEG(img)
		if err != nil {
			return err
		}
	}

	frameCopy := append([]byte(nil), data...)
	stream.mu.Lock()
	stream.frame.data = frameCopy
	stream.frame.width = config.Width
	stream.frame.height = config.Height
	stream.frame.sequence++
	stream.frame.source = source
	stream.frame.updatedAt = time.Now()
	stream.lastErr = nil
	notify := stream.notify
	stream.notify = make(chan struct{})
	close(notify)
	stream.mu.Unlock()
	return nil
}

func (stream *networkCameraStream) setError(err error) {
	if err == nil {
		return
	}
	stream.mu.Lock()
	stream.lastErr = err
	notify := stream.notify
	stream.notify = make(chan struct{})
	close(notify)
	stream.mu.Unlock()
}

func (stream *networkCameraStream) error() error {
	stream.mu.RLock()
	defer stream.mu.RUnlock()
	return stream.lastErr
}

func (stream *networkCameraStream) current(maxAge time.Duration) (pooledNetworkCameraFrame, bool) {
	stream.mu.RLock()
	defer stream.mu.RUnlock()
	if stream.frame.sequence == 0 || len(stream.frame.data) == 0 {
		return pooledNetworkCameraFrame{}, false
	}
	if maxAge > 0 && time.Since(stream.frame.updatedAt) > maxAge {
		return pooledNetworkCameraFrame{}, false
	}
	return stream.frame, true
}

func (stream *networkCameraStream) waitCurrent(ctx context.Context, maxAge, wait time.Duration) (pooledNetworkCameraFrame, error) {
	if frame, ok := stream.current(maxAge); ok {
		return frame, nil
	}
	if wait <= 0 {
		if err := stream.error(); err != nil {
			return pooledNetworkCameraFrame{}, err
		}
		return pooledNetworkCameraFrame{}, errors.New("连续流尚未输出画面")
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		stream.mu.RLock()
		if stream.frame.sequence > 0 && len(stream.frame.data) > 0 &&
			(maxAge <= 0 || time.Since(stream.frame.updatedAt) <= maxAge) {
			frame := stream.frame
			stream.mu.RUnlock()
			return frame, nil
		}
		notify := stream.notify
		lastErr := stream.lastErr
		stream.mu.RUnlock()
		if lastErr != nil {
			return pooledNetworkCameraFrame{}, lastErr
		}

		select {
		case <-ctx.Done():
			return pooledNetworkCameraFrame{}, ctx.Err()
		case <-timer.C:
			if lastErr != nil {
				return pooledNetworkCameraFrame{}, lastErr
			}
			return pooledNetworkCameraFrame{}, errors.New("等待连续流画面超时")
		case <-notify:
		}
	}
}


func (stream *networkCameraStream) waitNext(ctx context.Context, afterSequence uint64, wait time.Duration) (pooledNetworkCameraFrame, error) {
	if frame, ok := stream.current(0); ok && frame.sequence > afterSequence {
		return frame, nil
	}
	if wait <= 0 {
		wait = 500 * time.Millisecond
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		stream.mu.RLock()
		if stream.frame.sequence > afterSequence && len(stream.frame.data) > 0 {
			frame := stream.frame
			stream.mu.RUnlock()
			return frame, nil
		}
		notify := stream.notify
		lastErr := stream.lastErr
		stream.mu.RUnlock()

		select {
		case <-ctx.Done():
			return pooledNetworkCameraFrame{}, ctx.Err()
		case <-timer.C:
			if lastErr != nil {
				return pooledNetworkCameraFrame{}, lastErr
			}
			return pooledNetworkCameraFrame{}, errors.New("等待连续流下一帧超时")
		case <-notify:
		}
	}
}

func networkCameraContinuousMode(camera store.Camera) string {
	if camera.Kind != "network" {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(camera.Protocol)) {
	case "rtsp":
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(camera.StreamURL)), "rtsp://") ||
			strings.HasPrefix(strings.ToLower(strings.TrimSpace(camera.StreamURL)), "rtsps://") {
			return "rtsp"
		}
	case "mjpeg":
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(camera.StreamURL)), "http://") ||
			strings.HasPrefix(strings.ToLower(strings.TrimSpace(camera.StreamURL)), "https://") {
			return "mjpeg"
		}
	}
	return ""
}

func networkCameraStreamKey(camera store.Camera) string {
	return strings.Join([]string{
		camera.Protocol,
		camera.StreamURL,
		camera.Username,
		camera.Password,
		camera.AuthMode,
		strconv.Itoa(camera.TimeoutMS),
		strconv.FormatBool(camera.TLSInsecure),
	}, "\x00")
}

func (s *Server) ensureNetworkCameraStream(camera store.Camera) *networkCameraStream {
	if networkCameraContinuousMode(camera) == "" {
		return nil
	}

	key := networkCameraStreamKey(camera)
	s.networkCameraStreamMu.Lock()
	if s.networkCameraStreams == nil {
		s.networkCameraStreams = make(map[int64]*networkCameraStream)
	}
	if current := s.networkCameraStreams[camera.ID]; current != nil && current.key == key {
		s.networkCameraStreamMu.Unlock()
		return current
	}
	old := s.networkCameraStreams[camera.ID]
	ctx, cancel := context.WithCancel(context.Background())
	stream := newNetworkCameraStream(camera, key)
	stream.cancel = cancel
	s.networkCameraStreams[camera.ID] = stream
	s.networkCameraStreamMu.Unlock()

	if old != nil && old.cancel != nil {
		old.cancel()
	}
	go s.runNetworkCameraStream(ctx, stream)
	return stream
}

func (s *Server) stopNetworkCameraStream(cameraID int64) {
	s.networkCameraStreamMu.Lock()
	stream := s.networkCameraStreams[cameraID]
	delete(s.networkCameraStreams, cameraID)
	s.networkCameraStreamMu.Unlock()
	if stream != nil && stream.cancel != nil {
		stream.cancel()
	}
}

func (s *Server) stopAllNetworkCameraStreams() {
	s.networkCameraStreamMu.Lock()
	streams := make([]*networkCameraStream, 0, len(s.networkCameraStreams))
	for _, stream := range s.networkCameraStreams {
		streams = append(streams, stream)
	}
	s.networkCameraStreams = make(map[int64]*networkCameraStream)
	s.networkCameraStreamMu.Unlock()
	for _, stream := range streams {
		if stream != nil && stream.cancel != nil {
			stream.cancel()
		}
	}
}

func (s *Server) Close() {
	s.stopAllNetworkCameraStreams()
}

func (s *Server) networkCameraFrame(ctx context.Context, camera store.Camera) ([]byte, int, int, string, error) {
	stream := s.ensureNetworkCameraStream(camera)
	if stream != nil {
		if frame, ok := stream.current(networkCameraStreamFreshFor); ok {
			return frame.data, frame.width, frame.height, frame.source, nil
		}
		if strings.TrimSpace(camera.SnapshotURL) == "" {
			wait := time.Duration(camera.TimeoutMS) * time.Millisecond
			if wait <= 0 {
				wait = 2 * time.Second
			}
			if wait > 2500*time.Millisecond {
				wait = 2500 * time.Millisecond
			}
			frame, err := stream.waitCurrent(ctx, networkCameraStreamFreshFor, wait)
			if err == nil {
				return frame.data, frame.width, frame.height, frame.source, nil
			}
		}
	}

	if strings.TrimSpace(camera.SnapshotURL) != "" || strings.EqualFold(camera.Protocol, "http_snapshot") ||
		(strings.EqualFold(camera.Protocol, "mjpeg") && strings.TrimSpace(camera.StreamURL) != "") {
		frame, width, height, err := s.cachedNetworkCameraFrame(ctx, camera)
		if err == nil {
			return frame, width, height, "snapshot-fallback", nil
		}
		if stream != nil {
			if streamErr := stream.error(); streamErr != nil {
				return nil, 0, 0, "", fmt.Errorf("连续流不可用（%v），HTTP抓图回退也失败: %w", streamErr, err)
			}
		}
		return nil, 0, 0, "", err
	}

	if stream != nil {
		if streamErr := stream.error(); streamErr != nil {
			return nil, 0, 0, "", streamErr
		}
		return nil, 0, 0, "", errors.New("连续流尚未准备好，且未配置HTTP抓图回退")
	}
	return nil, 0, 0, "", errors.New("没有可用的摄像头视频源")
}

func (s *Server) runNetworkCameraStream(ctx context.Context, stream *networkCameraStream) {
	retryDelay := networkCameraStreamRetryMin
	for {
		if ctx.Err() != nil {
			return
		}

		started := time.Now()
		var err error
		switch networkCameraContinuousMode(stream.camera) {
		case "rtsp":
			err = s.consumeRTSPCameraStream(ctx, stream)
		case "mjpeg":
			err = s.consumeMJPEGCameraStream(ctx, stream)
		default:
			err = errors.New("没有可用的连续流配置")
		}
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			err = errors.New("连续流连接已结束")
		}
		stream.setError(err)
		s.logger.Warn("network camera continuous stream reconnecting",
			"camera_id", stream.camera.ID,
			"camera", stream.camera.Name,
			"mode", networkCameraContinuousMode(stream.camera),
			"retry_after", retryDelay,
			"error", err,
		)

		if time.Since(started) >= 10*time.Second {
			retryDelay = networkCameraStreamRetryMin
		} else if retryDelay < networkCameraStreamRetryMax {
			retryDelay *= 2
			if retryDelay > networkCameraStreamRetryMax {
				retryDelay = networkCameraStreamRetryMax
			}
		}

		timer := time.NewTimer(retryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func continuousCameraHTTPClient(camera store.Camera) *http.Client {
	timeout := time.Duration(camera.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	transport := cameraHTTPTransport(camera.TLSInsecure).Clone()
	transport.DialContext = (&net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = timeout
	transport.ResponseHeaderTimeout = timeout
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("摄像头重定向次数过多")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return errors.New("摄像头重定向到了不支持的协议")
			}
			return nil
		},
	}
}

func (s *Server) consumeMJPEGCameraStream(ctx context.Context, stream *networkCameraStream) error {
	client := continuousCameraHTTPClient(stream.camera)
	response, _, err := doCameraRequest(ctx, client, stream.camera, stream.camera.StreamURL)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("MJPEG连续流返回 HTTP %d", response.StatusCode)
	}
	return readJPEGSequence(ctx, response.Body, func(frame []byte) error {
		return stream.publish(frame, "mjpeg")
	})
}

func findFFmpeg() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("FACESIGN_FFMPEG")); configured != "" {
		info, err := os.Stat(configured)
		if err != nil || info.IsDir() {
			return "", fmt.Errorf("FACESIGN_FFMPEG 指定的 FFmpeg 不存在: %s", configured)
		}
		return configured, nil
	}

	name := "ffmpeg"
	if runtime.GOOS == "windows" {
		name = "ffmpeg.exe"
	}
	if executable, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(executable), name)
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	if candidate, err := exec.LookPath(name); err == nil {
		return candidate, nil
	}
	return "", errors.New("未找到 FFmpeg；RTSP 连续流需要发布包中的 ffmpeg.exe，系统会暂时使用 HTTP 抓图回退")
}

func ffmpegRTSPInputURL(camera store.Camera) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(camera.StreamURL))
	if err != nil || parsed.Host == "" {
		return "", errors.New("RTSP 视频流地址格式不正确")
	}
	if strings.TrimSpace(camera.Username) != "" {
		parsed.User = url.UserPassword(camera.Username, camera.Password)
	}
	return parsed.String(), nil
}

func ffmpegRTSPArgs(camera store.Camera, inputURL string) []string {
	timeout := time.Duration(camera.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return []string{
		"-hide_banner",
		"-loglevel", "error",
		"-nostdin",
		"-fflags", "nobuffer",
		"-flags", "low_delay",
		"-max_delay", "500000",
		"-rtsp_transport", "tcp",
		"-rw_timeout", strconv.FormatInt(timeout.Microseconds(), 10),
		"-i", inputURL,
		"-map", "0:v:0",
		"-an",
		"-sn",
		"-dn",
		"-c:v", "mjpeg",
		"-q:v", "5",
		"-f", "image2pipe",
		"pipe:1",
	}
}

func (s *Server) consumeRTSPCameraStream(ctx context.Context, stream *networkCameraStream) error {
	ffmpegPath, err := findFFmpeg()
	if err != nil {
		return err
	}
	inputURL, err := ffmpegRTSPInputURL(stream.camera)
	if err != nil {
		return err
	}

	command := exec.CommandContext(ctx, ffmpegPath, ffmpegRTSPArgs(stream.camera, inputURL)...)
	command.Stderr = io.Discard
	command.WaitDelay = 2 * time.Second
	stdout, err := command.StdoutPipe()
	if err != nil {
		return fmt.Errorf("创建 FFmpeg 输出管道失败: %w", err)
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("启动 FFmpeg RTSP 解码失败: %w", err)
	}

	readErr := readJPEGSequence(ctx, stdout, func(frame []byte) error {
		return stream.publish(frame, "rtsp")
	})
	if readErr != nil && ctx.Err() == nil && command.Process != nil {
		_ = command.Process.Kill()
	}
	waitErr := command.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return fmt.Errorf("读取 RTSP 解码帧失败: %w", readErr)
	}
	if waitErr != nil {
		return fmt.Errorf("FFmpeg RTSP 解码已退出: %w", waitErr)
	}
	return errors.New("RTSP连续流已结束")
}

func readJPEGSequence(ctx context.Context, reader io.Reader, onFrame func([]byte) error) error {
	const chunkSize = 64 * 1024
	startMarker := []byte{0xff, 0xd8}
	endMarker := []byte{0xff, 0xd9}
	chunk := make([]byte, chunkSize)
	pending := make([]byte, 0, 1024*1024)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, readErr := reader.Read(chunk)
		if n > 0 {
			pending = append(pending, chunk[:n]...)
			for {
				start := bytes.Index(pending, startMarker)
				if start < 0 {
					if len(pending) > 1 {
						pending = append(pending[:0], pending[len(pending)-1])
					}
					break
				}
				if start > 0 {
					pending = pending[start:]
				}
				endOffset := bytes.Index(pending[2:], endMarker)
				if endOffset < 0 {
					if len(pending) > maxContinuousFrameBytes {
						return errors.New("连续流中单个JPEG帧超过16MB限制")
					}
					break
				}
				end := 2 + endOffset + 2
				frame := append([]byte(nil), pending[:end]...)
				pending = pending[end:]
				if err := onFrame(frame); err != nil {
					return err
				}
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return io.EOF
			}
			return readErr
		}
	}
}

func networkCameraProbeTimeout(camera store.Camera) time.Duration {
	timeout := time.Duration(camera.TimeoutMS)*time.Millisecond + 3*time.Second
	if timeout < 4*time.Second {
		timeout = 4 * time.Second
	}
	if timeout > 10*time.Second {
		timeout = 10 * time.Second
	}
	return timeout
}

func (s *Server) probeNetworkCameraPrimaryFrame(ctx context.Context, camera store.Camera) ([]byte, int, int, string, error) {
	mode := networkCameraContinuousMode(camera)
	if mode == "" {
		return nil, 0, 0, "", errors.New("当前配置没有连续流主通道")
	}

	probeCtx, cancel := context.WithTimeout(ctx, networkCameraProbeTimeout(camera))
	defer cancel()
	stream := newNetworkCameraStream(camera, "probe")
	done := make(chan error, 1)
	go func() {
		var err error
		if mode == "rtsp" {
			err = s.consumeRTSPCameraStream(probeCtx, stream)
		} else {
			err = s.consumeMJPEGCameraStream(probeCtx, stream)
		}
		stream.setError(err)
		done <- err
	}()

	frame, err := stream.waitCurrent(probeCtx, 0, networkCameraProbeTimeout(camera))
	if err == nil {
		cancel()
		return frame.data, frame.width, frame.height, frame.source, nil
	}
	select {
	case runErr := <-done:
		if runErr != nil {
			return nil, 0, 0, "", runErr
		}
	default:
	}
	return nil, 0, 0, "", err
}
