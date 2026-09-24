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
	Mode string `json:"mode,omitempty"`
}

type webRTCH264Encoder struct {
	Name string
	Mode string
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
	ffmpegPath, err := findFFmpeg()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("WebRTC 实时预览不可用: %w", err))
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
	source, err := resolveRTSPSourceForPurpose(probeCtx, camera, true, "preview")
	probeCancel()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("WebRTC 实时预览不可用: %w", err))
		return
	}

	encoder := webRTCH264Encoder{Mode: "WebRTC H.264直通"}
	if source.Codec == "hevc" {
		encoderCtx, encoderCancel := context.WithTimeout(r.Context(), 10*time.Second)
		encoder, err = selectWebRTCH264Encoder(encoderCtx, ffmpegPath)
		encoderCancel()
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, fmt.Errorf("WebRTC H.265 转码不可用: %w", err))
			return
		}
	} else if source.Codec != "h264" {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("WebRTC 实时预览暂不支持 RTSP 编码 %q", source.Codec))
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

		if err := streamRTSPH264ToWebRTC(streamCtx, camera, videoTrack, source, encoder); err != nil && streamCtx.Err() == nil {
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
		Mode: encoder.Mode,
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

func selectWebRTCH264Encoder(ctx context.Context, ffmpegPath string) (webRTCH264Encoder, error) {
	candidates := []webRTCH264Encoder{
		{Name: "h264_nvenc", Mode: "WebRTC H.265→H.264硬件转码(NVIDIA)"},
		{Name: "h264_qsv", Mode: "WebRTC H.265→H.264硬件转码(Intel)"},
		{Name: "h264_amf", Mode: "WebRTC H.265→H.264硬件转码(AMD)"},
		{Name: "h264_mf", Mode: "WebRTC H.265→H.264 Windows转码"},
		{Name: "libopenh264", Mode: "WebRTC H.265→H.264软件转码"},
	}
	var failures []string
	for _, candidate := range candidates {
		probeCtx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
		err := probeWebRTCH264Encoder(probeCtx, ffmpegPath, candidate.Name)
		cancel()
		if err == nil {
			return candidate, nil
		}
		failures = append(failures, candidate.Name)
		if ctx.Err() != nil {
			break
		}
	}
	return webRTCH264Encoder{}, fmt.Errorf("FFmpeg 没有可用的 H.264 转码编码器（已尝试 %s）", strings.Join(failures, "、"))
}

func probeWebRTCH264Encoder(ctx context.Context, ffmpegPath, encoder string) error {
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-nostdin",
		"-f", "lavfi",
		"-i", "color=c=black:s=320x180:r=25",
		"-frames:v", "2",
		"-vf", "format=yuv420p",
		"-c:v", encoder,
		"-b:v", "500k",
		"-g", "25",
		"-bf", "0",
		"-f", "h264",
		"-",
	}
	command := exec.CommandContext(ctx, ffmpegPath, args...)
	command.Stdout = io.Discard
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%s: %s", encoder, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func webRTCH264OutputArgs(camera store.Camera, source resolvedRTSPSource, encoder webRTCH264Encoder) []string {
	if source.Codec == "h264" {
		return []string{
			"-c:v", "copy",
			"-bsf:v", "h264_mp4toannexb,dump_extra=freq=keyframe",
		}
	}

	width := camera.Width
	height := camera.Height
	if width <= 0 {
		width = 1280
	}
	if height <= 0 {
		height = 720
	}
	bitrate := "2500k"
	maxrate := "3000k"
	bufsize := "1500k"
	if width >= 1600 || height >= 900 {
		bitrate = "4000k"
		maxrate = "4500k"
		bufsize = "2200k"
	}
	scale := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease:force_divisible_by=2,format=yuv420p", width, height)
	return []string{
		"-vf", scale,
		"-c:v", encoder.Name,
		"-b:v", bitrate,
		"-maxrate", maxrate,
		"-bufsize", bufsize,
		"-g", "30",
		"-bf", "0",
		"-fps_mode", "passthrough",
		"-bsf:v", "h264_mp4toannexb,dump_extra=freq=keyframe",
	}
}

func streamRTSPH264ToWebRTC(ctx context.Context, camera store.Camera, track *webrtc.TrackLocalStaticRTP, source resolvedRTSPSource, encoder webRTCH264Encoder) error {
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
	}
	args = append(args, webRTCH264OutputArgs(camera, source, encoder)...)
	args = append(args,
		"-f", "rtp",
		"-payload_type", "96",
		target,
	)

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
