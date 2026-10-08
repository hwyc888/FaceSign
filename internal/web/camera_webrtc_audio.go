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

type cameraWebRTCAudioSignal struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
	Mode string `json:"mode,omitempty"`
}

func cameraWebRTCAudioCodecCapability() webrtc.RTPCodecCapability {
	return webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2, SDPFmtpLine: "minptime=10;useinbandfec=1"}
}

func cameraWebRTCAudioFFmpegArgs(inputArgs []string, target string) []string {
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin"}
	args = append(args, inputArgs...)
	return append(args, "-map", "0:a:0", "-vn", "-sn", "-dn", "-c:a", "libopus", "-ar", "48000", "-ac", "2", "-b:a", "64k", "-f", "rtp", "-payload_type", "111", target)
}

func streamFFmpegAudioRTP(ctx context.Context, ffmpegPath string, inputArgs []string, track *webrtc.TrackLocalStaticRTP) error {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil { return fmt.Errorf("创建音频RTP本地接收端口失败: %w", err) }
	defer conn.Close()
	target := fmt.Sprintf("rtp://127.0.0.1:%d?pkt_size=1200", conn.LocalAddr().(*net.UDPAddr).Port)
	command := exec.CommandContext(ctx, ffmpegPath, cameraWebRTCAudioFFmpegArgs(inputArgs, target)...)
	command.Stdout = io.Discard
	var stderr bytes.Buffer
	command.Stderr = &stderr
	command.WaitDelay = 2 * time.Second
	if err := command.Start(); err != nil { return fmt.Errorf("启动摄像头音频转发失败: %w", err) }
	waitCh := make(chan error, 1)
	go func() { waitCh <- command.Wait() }()
	finished := false
	defer func() {
		if finished || command.Process == nil { return }
		_ = command.Process.Kill()
		select { case <-waitCh: case <-time.After(2 * time.Second): }
	}()
	packet := make([]byte, 64*1024)
	for {
		select {
		case <-ctx.Done(): return ctx.Err()
		case waitErr := <-waitCh:
			finished = true
			detail := strings.TrimSpace(stderr.String())
			if detail == "" { detail = fmt.Sprint(waitErr) }
			if len(detail) > 500 { detail = detail[len(detail)-500:] }
			return fmt.Errorf("摄像头音频转发已停止: %s", detail)
		default:
		}
		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		n, _, readErr := conn.ReadFromUDP(packet)
		if readErr != nil {
			if networkErr, ok := readErr.(net.Error); ok && networkErr.Timeout() { continue }
			if ctx.Err() != nil { return ctx.Err() }
			return fmt.Errorf("读取摄像头音频RTP失败: %w", readErr)
		}
		if n < 12 { continue }
		if _, err := track.Write(packet[:n]); err != nil {
			if ctx.Err() != nil { return ctx.Err() }
			return fmt.Errorf("发送摄像头音频RTP失败: %w", err)
		}
	}
}

func streamRTSPAudioToWebRTC(ctx context.Context, camera store.Camera, source webRTCRTSPSource, track *webrtc.TrackLocalStaticRTP) error {
	ffmpegPath, err := findFFmpeg()
	if err != nil { return err }
	timeout := time.Duration(camera.TimeoutMS) * time.Millisecond
	if timeout <= 0 { timeout = 3 * time.Second }
	inputArgs := []string{"-fflags", "nobuffer", "-flags", "low_delay", "-max_delay", "500000", "-rtsp_transport", source.Transport, "-timeout", strconv.FormatInt(timeout.Microseconds(), 10), "-i", source.URL}
	if err := streamFFmpegAudioRTP(ctx, ffmpegPath, inputArgs, track); err != nil { return errors.New(sanitizeRTSPDiagnostic(err.Error(), camera, source.URL)) }
	return nil
}

func streamHostAudioToWebRTC(ctx context.Context, device hostCameraDevice, track *webrtc.TrackLocalStaticRTP) error {
	ffmpegPath, err := findFFmpeg()
	if err != nil { return err }
	return streamFFmpegAudioRTP(ctx, ffmpegPath, []string{"-f", "dshow", "-thread_queue_size", "128", "-i", "audio=" + device.ID}, track)
}

func (s *Server) cameraWebRTCAudioOffer(w http.ResponseWriter, r *http.Request, camera store.Camera) {
	if r.Method != http.MethodPost { methodNotAllowed(w); return }
	var offer cameraWebRTCAudioSignal
	if err := decodeJSON(r, &offer); err != nil { writeError(w, http.StatusBadRequest, err); return }
	if strings.ToLower(strings.TrimSpace(offer.Type)) != "offer" || strings.TrimSpace(offer.SDP) == "" { writeError(w, http.StatusBadRequest, errors.New("WebRTC音频offer不正确")); return }

	mode := ""
	var streamAudio func(context.Context, *webrtc.TrackLocalStaticRTP) error
	switch {
	case camera.Kind == "network" && strings.EqualFold(strings.TrimSpace(camera.Protocol), "rtsp"):
		probeCtx, cancel := context.WithTimeout(r.Context(), cameraWebRTCProbeTimeout(camera)+4*time.Second)
		source, err := resolveWebRTCRTSPSource(probeCtx, camera)
		cancel()
		if err != nil { writeError(w, http.StatusServiceUnavailable, fmt.Errorf("读取RTSP音频失败: %w", err)); return }
		if !source.HasAudio { writeError(w, http.StatusServiceUnavailable, errors.New("当前RTSP码流没有检测到音频轨；请先在摄像头中启用音频并确认主/子码流包含声音")); return }
		mode = "RTSP摄像头音频"
		streamAudio = func(ctx context.Context, track *webrtc.TrackLocalStaticRTP) error { return streamRTSPAudioToWebRTC(ctx, camera, source, track) }
	case camera.Kind == "local":
		audioCtx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		device, err := selectHostAudioDevice(audioCtx, camera)
		cancel()
		if err != nil { writeError(w, http.StatusServiceUnavailable, fmt.Errorf("读取FaceSign主机音频失败: %w", err)); return }
		mode = "FaceSign主机音频：" + device.Name
		streamAudio = func(ctx context.Context, track *webrtc.TrackLocalStaticRTP) error { return streamHostAudioToWebRTC(ctx, device, track) }
	default:
		writeError(w, http.StatusBadRequest, errors.New("声音监听仅支持RTSP网络摄像头和FaceSign主机USB摄像头"))
		return
	}

	peer, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil { writeError(w, http.StatusInternalServerError, fmt.Errorf("创建WebRTC音频连接失败: %w", err)); return }
	audioTrack, err := webrtc.NewTrackLocalStaticRTP(cameraWebRTCAudioCodecCapability(), "audio", "facesign-camera-audio")
	if err != nil { _ = peer.Close(); writeError(w, http.StatusInternalServerError, fmt.Errorf("创建WebRTC音频轨失败: %w", err)); return }
	sender, err := peer.AddTrack(audioTrack)
	if err != nil { _ = peer.Close(); writeError(w, http.StatusInternalServerError, fmt.Errorf("添加WebRTC音频轨失败: %w", err)); return }
	go drainCameraWebRTCRTCP(sender)

	streamCtx, cancelStream := context.WithCancel(context.Background())
	connected := make(chan struct{})
	var connectedOnce sync.Once
	peer.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		switch state {
		case webrtc.PeerConnectionStateConnected: connectedOnce.Do(func() { close(connected) })
		case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed: cancelStream()
		}
	})

	if err := peer.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offer.SDP}); err != nil { cancelStream(); _ = peer.Close(); writeError(w, http.StatusBadRequest, fmt.Errorf("应用WebRTC音频offer失败: %w", err)); return }
	answer, err := peer.CreateAnswer(nil)
	if err != nil { cancelStream(); _ = peer.Close(); writeError(w, http.StatusInternalServerError, fmt.Errorf("创建WebRTC音频answer失败: %w", err)); return }
	gatherComplete := webrtc.GatheringCompletePromise(peer)
	if err := peer.SetLocalDescription(answer); err != nil { cancelStream(); _ = peer.Close(); writeError(w, http.StatusInternalServerError, fmt.Errorf("设置WebRTC音频answer失败: %w", err)); return }
	select {
	case <-r.Context().Done(): cancelStream(); _ = peer.Close(); return
	case <-gatherComplete:
	case <-time.After(5 * time.Second): cancelStream(); _ = peer.Close(); writeError(w, http.StatusGatewayTimeout, errors.New("WebRTC音频ICE候选收集超时")); return
	}
	local := peer.LocalDescription()
	if local == nil { cancelStream(); _ = peer.Close(); writeError(w, http.StatusInternalServerError, errors.New("WebRTC音频answer未生成")); return }

	go func() {
		defer cancelStream()
		defer peer.Close()
		select {
		case <-streamCtx.Done(): return
		case <-connected:
		case <-time.After(12 * time.Second): s.logger.Warn("camera WebRTC audio connection timeout", "camera_id", camera.ID, "camera", camera.Name); return
		}
		if err := streamAudio(streamCtx, audioTrack); err != nil && streamCtx.Err() == nil { s.logger.Warn("camera WebRTC audio stopped", "camera_id", camera.ID, "camera", camera.Name, "mode", mode, "error", err) }
	}()

	writeJSON(w, http.StatusOK, cameraWebRTCAudioSignal{Type: "answer", SDP: local.SDP, Mode: mode})
}
