package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hwyc888/FaceSign/internal/store"
	"github.com/pion/webrtc/v4"
)

type cameraWebRTCSignal struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

func (s *Server) cameraWebRTCOffer(w http.ResponseWriter, r *http.Request, camera store.Camera) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if camera.Kind != "network" || strings.ToLower(strings.TrimSpace(camera.Protocol)) != "rtsp" {
		writeError(w, http.StatusBadRequest, errors.New("WebRTC H.264 预览仅用于 RTSP 网络摄像头"))
		return
	}
	if _, err := findFFmpeg(); err != nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("WebRTC H.264 预览不可用: %w", err))
		return
	}

	var offer cameraWebRTCSignal
	if err := decodeJSON(r, &offer); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.ToLower(strings.TrimSpace(offer.Type)) != "offer" || strings.TrimSpace(offer.SDP) == "" {
		writeError(w, http.StatusBadRequest, errors.New("WebRTC offer 不正确"))
		return
	}

	probeCtx, probeCancel := context.WithTimeout(r.Context(), cameraWebRTCProbeTimeout(camera)+4*time.Second)
	source, err := resolveRTSPSource(probeCtx, camera, true)
	probeCancel()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("WebRTC H.264 预览不可用: %w", err))
		return
	}

	peer, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("创建 WebRTC 连接失败: %w", err))
		return
	}

	videoTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeH264,
			ClockRate: 90000,
		},
		"video",
		"facesign-camera",
	)
	if err != nil {
		_ = peer.Close()
		writeError(w, http.StatusInternalServerError, fmt.Errorf("创建 WebRTC H.264 视频轨失败: %w", err))
		return
	}

	sender, err := peer.AddTrack(videoTrack)
	if err != nil {
		_ = peer.Close()
		writeError(w, http.StatusInternalServerError, fmt.Errorf("添加 WebRTC 视频轨失败: %w", err))
		return
	}
	go drainCameraWebRTCRTCP(sender)

	streamCtx, cancelStream := context.WithCancel(context.Background())
	connected := make(chan struct{})
	var connectedOnce sync.Once
	peer.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		switch state {
		case webrtc.PeerConnectionStateConnected:
			connectedOnce.Do(func() { close(connected) })
		case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed:
			cancelStream()
		}
	})

	if err := peer.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offer.SDP,
	}); err != nil {
		cancelStream()
		_ = peer.Close()
		writeError(w, http.StatusBadRequest, fmt.Errorf("应用 WebRTC offer 失败: %w", err))
		return
	}

	answer, err := peer.CreateAnswer(nil)
	if err != nil {
		cancelStream()
		_ = peer.Close()
		writeError(w, http.StatusInternalServerError, fmt.Errorf("创建 WebRTC answer 失败: %w", err))
		return
	}
	gatherComplete := webrtc.GatheringCompletePromise(peer)
	if err := peer.SetLocalDescription(answer); err != nil {
		cancelStream()
		_ = peer.Close()
		writeError(w, http.StatusInternalServerError, fmt.Errorf("设置 WebRTC answer 失败: %w", err))
		return
	}

	select {
	case <-r.Context().Done():
		cancelStream()
		_ = peer.Close()
		return
	case <-gatherComplete:
	case <-time.After(5 * time.Second):
		cancelStream()
		_ = peer.Close()
		writeError(w, http.StatusGatewayTimeout, errors.New("WebRTC ICE 候选收集超时"))
		return
	}

	local := peer.LocalDescription()
	if local == nil {
		cancelStream()
		_ = peer.Close()
		writeError(w, http.StatusInternalServerError, errors.New("WebRTC answer 未生成"))
		return
	}

	go func() {
		defer cancelStream()
		defer peer.Close()

		select {
		case <-streamCtx.Done():
			return
		case <-connected:
		case <-time.After(12 * time.Second):
			s.logger.Warn("camera WebRTC connection timeout", "camera_id", camera.ID, "camera", camera.Name)
			return
		}

		if err := streamRTSPH264ToWebRTC(streamCtx, camera, videoTrack, source); err != nil && streamCtx.Err() == nil {
			s.logger.Warn("camera WebRTC H264 stream stopped",
				"camera_id", camera.ID,
				"camera", camera.Name,
				"error", err,
			)
		}
	}()

	writeJSON(w, http.StatusOK, cameraWebRTCSignal{
		Type: "answer",
		SDP:  local.SDP,
	})
}

func drainCameraWebRTCRTCP(sender *webrtc.RTPSender) {
	buf := make([]byte, 1500)
	for {
		if _, _, err := sender.Read(buf); err != nil {
			return
		}
	}
}

func cameraWebRTCProbeTimeout(camera store.Camera) time.Duration {
	timeout := time.Duration(camera.TimeoutMS) * time.Millisecond
	if timeout < 3*time.Second {
		timeout = 3 * time.Second
	}
	if timeout > 8*time.Second {
		timeout = 8 * time.Second
	}
	return timeout
}

func probeRTSPH264(ctx context.Context, camera store.Camera) error {
	_, err := resolveRTSPSource(ctx, camera, true)
	return err
}

func streamRTSPH264ToWebRTC(ctx context.Context, camera store.Camera, track *webrtc.TrackLocalStaticRTP, source resolvedRTSPSource) error {
	ffmpegPath, err := findFFmpeg()
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		return fmt.Errorf("创建 WebRTC RTP 本地接收端口失败: %w", err)
	}
	defer conn.Close()

	port := conn.LocalAddr().(*net.UDPAddr).Port
	timeout := time.Duration(camera.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	target := fmt.Sprintf("rtp://127.0.0.1:%d?pkt_size=1200", port)
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-nostdin",
		"-fflags", "nobuffer",
		"-flags", "low_delay",
		"-max_delay", "500000",
		"-rtsp_transport", source.Transport,
		"-timeout", strconv.FormatInt(timeout.Microseconds(), 10),
		"-i", source.URL,
		"-map", "0:v:0",
		"-an",
		"-sn",
		"-dn",
		"-c:v", "copy",
		"-bsf:v", "h264_mp4toannexb,dump_extra=freq=keyframe",
		"-f", "rtp",
		"-payload_type", "96",
		target,
	}

	command := exec.CommandContext(ctx, ffmpegPath, args...)
	command.Stdout = io.Discard
	var stderr bytes.Buffer
	command.Stderr = &stderr
	command.WaitDelay = 2 * time.Second
	if err := command.Start(); err != nil {
		return fmt.Errorf("启动 WebRTC H.264 RTP 转发失败: %w", err)
	}

	waitCh := make(chan error, 1)
	go func() { waitCh <- command.Wait() }()

	finished := false
	defer func() {
		if finished || command.Process == nil {
			return
		}
		_ = command.Process.Kill()
		select {
		case <-waitCh:
		case <-time.After(2 * time.Second):
		}
	}()

	packet := make([]byte, 64*1024)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case waitErr := <-waitCh:
			finished = true
			if waitErr == nil {
				return errors.New("WebRTC H.264 RTP 转发已结束")
			}
			detail := classifyRTSPProbeFailure(stderr.String(), waitErr, camera, source.URL)
			return fmt.Errorf("WebRTC H.264 RTP 转发退出: %s", detail)
		default:
		}

		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		n, _, readErr := conn.ReadFromUDP(packet)
		if readErr != nil {
			if networkErr, ok := readErr.(net.Error); ok && networkErr.Timeout() {
				continue
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("读取 WebRTC RTP 数据失败: %w", readErr)
		}
		if n < 12 {
			continue
		}
		if _, err := track.Write(packet[:n]); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("发送 WebRTC RTP 数据失败: %w", err)
		}
	}
}
