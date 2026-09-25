package web

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/hwyc888/FaceSign/internal/store"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

type webRTCRTSPSource struct {
	resolvedRTSPSource
	H264 *format.H264
	H265 *format.H265
}

func webRTCH264CodecCapability(source webRTCRTSPSource) webrtc.RTPCodecCapability {
	capability := webrtc.RTPCodecCapability{
		MimeType:  webrtc.MimeTypeH264,
		ClockRate: 90000,
	}
	if source.Codec != "h264" || source.H264 == nil {
		return capability
	}

	params := source.H264.FMTP()
	profileLevelID := strings.ToLower(strings.TrimSpace(params["profile-level-id"]))
	if profileLevelID == "" {
		return capability
	}
	capability.SDPFmtpLine = fmt.Sprintf(
		"level-asymmetry-allowed=1;packetization-mode=%d;profile-level-id=%s",
		source.H264.PacketizationMode,
		profileLevelID,
	)
	return capability
}

func webRTCH265CodecCapability() webrtc.RTPCodecCapability {
	return webrtc.RTPCodecCapability{
		MimeType:  webrtc.MimeTypeH265,
		ClockRate: 90000,
	}
}

func webRTSPClientTimeout(camera store.Camera) time.Duration {
	timeout := time.Duration(camera.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	if timeout < time.Second {
		timeout = time.Second
	}
	if timeout > 5*time.Second {
		timeout = 5 * time.Second
	}
	return timeout
}

func newWebRTSPClient(ctx context.Context, target *base.URL, camera store.Camera) *gortsplib.Client {
	timeout := webRTSPClientTimeout(camera)
	protocol := gortsplib.ProtocolTCP
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	return &gortsplib.Client{
		Scheme:       target.Scheme,
		Host:         target.Host,
		ReadTimeout:  timeout,
		WriteTimeout: timeout,
		Protocol:     &protocol,
		UserAgent:    "FaceSign/RTSP",
		DialContext: func(_ context.Context, network, address string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, address)
		},
	}
}

func resolveWebRTCRTSPSource(ctx context.Context, camera store.Camera) (webRTCRTSPSource, error) {
	candidates, err := rtspCandidatesForPurpose(camera, "preview")
	if err != nil {
		return webRTCRTSPSource{}, err
	}

	diagnostics := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			break
		}
		source, err := describeWebRTSPCandidate(ctx, camera, candidate)
		if err == nil {
			return source, nil
		}
		diagnostics = append(diagnostics, candidate.Label+"/TCP: "+classifyWebRTSPFailure(err, camera, candidate.URL))
	}

	if len(diagnostics) == 0 {
		if err := ctx.Err(); err != nil {
			return webRTCRTSPSource{}, fmt.Errorf("RTSP 连接超时: %w", err)
		}
		return webRTCRTSPSource{}, errors.New("RTSP 连接失败，未收到摄像头响应")
	}
	return webRTCRTSPSource{}, errors.New(bestRTSPDiagnostic(diagnostics))
}

func describeWebRTSPCandidate(ctx context.Context, camera store.Camera, candidate rtspCandidate) (webRTCRTSPSource, error) {
	target, err := base.ParseURL(candidate.URL)
	if err != nil {
		return webRTCRTSPSource{}, fmt.Errorf("RTSP 地址格式不正确: %w", err)
	}

	client := newWebRTSPClient(ctx, target, camera)
	if err := client.Start(); err != nil {
		return webRTCRTSPSource{}, err
	}
	defer client.Close()

	desc, _, err := client.Describe(target)
	if err != nil {
		return webRTCRTSPSource{}, err
	}

	var h264 *format.H264
	if desc.FindFormat(&h264) != nil {
		return webRTCRTSPSource{
			resolvedRTSPSource: resolvedRTSPSource{
				URL:       candidate.URL,
				Transport: "tcp",
				Codec:     "h264",
				Label:     candidate.Label,
			},
			H264: h264,
		}, nil
	}

	var h265 *format.H265
	if desc.FindFormat(&h265) != nil {
		return webRTCRTSPSource{
			resolvedRTSPSource: resolvedRTSPSource{
				URL:       candidate.URL,
				Transport: "tcp",
				Codec:     "hevc",
				Label:     candidate.Label,
			},
			H265: h265,
		}, nil
	}

	return webRTCRTSPSource{}, errors.New("RTSP 已连接，但视频不是 H.264/H.265")
}

func classifyWebRTSPFailure(err error, camera store.Camera, rawURL string) string {
	if err == nil {
		return ""
	}
	text := sanitizeRTSPDiagnostic(err.Error(), camera, rawURL)
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "401"), strings.Contains(lower, "unauthorized"):
		return "RTSP 认证失败（401），请检查用户名、密码或摄像头 RTSP 认证方式"
	case strings.Contains(lower, "404"), strings.Contains(lower, "not found"):
		return "RTSP 路径不存在（404）"
	case strings.Contains(lower, "connection refused"):
		return "RTSP 端口拒绝连接，请检查摄像头 RTSP 端口是否已启用"
	case strings.Contains(lower, "timeout"), strings.Contains(lower, "timed out"), strings.Contains(lower, "deadline exceeded"):
		return "RTSP 连接超时，请检查 RTSP 端口、网络或摄像头服务"
	case strings.Contains(lower, "no route"), strings.Contains(lower, "unreachable"):
		return "无法到达摄像头 RTSP 网络"
	default:
		if len(text) > 180 {
			text = text[:180] + "…"
		}
		return text
	}
}

func streamRTSPH264DirectToWebRTC(
	ctx context.Context,
	camera store.Camera,
	track *webrtc.TrackLocalStaticRTP,
	source webRTCRTSPSource,
) error {
	target, err := base.ParseURL(source.URL)
	if err != nil {
		return fmt.Errorf("RTSP 地址格式不正确: %w", err)
	}

	client := newWebRTSPClient(ctx, target, camera)
	if err := client.Start(); err != nil {
		return fmt.Errorf("连接 RTSP H.264 失败: %w", err)
	}
	defer client.Close()

	desc, _, err := client.Describe(target)
	if err != nil {
		return fmt.Errorf("读取 RTSP H.264 描述失败: %w", err)
	}

	var h264 *format.H264
	h264Media := desc.FindFormat(&h264)
	if h264Media == nil {
		return errors.New("RTSP 码流已不再是 H.264")
	}
	if err := client.SetupAll(desc.BaseURL, []*description.Media{h264Media}); err != nil {
		return fmt.Errorf("建立 RTSP H.264 RTP 通道失败: %w", err)
	}

	writeErr := make(chan error, 1)
	client.OnPacketRTP(h264Media, h264, func(pkt *rtp.Packet) {
		if err := track.WriteRTP(pkt); err != nil {
			select {
			case writeErr <- err:
			default:
			}
		}
	})

	if _, err := client.Play(nil); err != nil {
		return fmt.Errorf("启动 RTSP H.264 播放失败: %w", err)
	}

	waitErr := make(chan error, 1)
	go func() {
		waitErr <- client.Wait()
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-writeErr:
		return fmt.Errorf("发送原始 H.264 RTP 到 WebRTC 失败: %w", err)
	case err := <-waitErr:
		if err == nil {
			return errors.New("RTSP H.264 原码流已结束")
		}
		return fmt.Errorf("RTSP H.264 原码流中断: %w", err)
	}
}

func probeRTSPH265Direct(
	ctx context.Context,
	camera store.Camera,
	source webRTCRTSPSource,
) error {
	target, err := base.ParseURL(source.URL)
	if err != nil {
		return fmt.Errorf("RTSP 地址格式不正确: %w", err)
	}

	client := newWebRTSPClient(ctx, target, camera)
	if err := client.Start(); err != nil {
		return fmt.Errorf("连接 RTSP H.265 失败: %w", err)
	}
	defer client.Close()

	desc, _, err := client.Describe(target)
	if err != nil {
		return fmt.Errorf("读取 RTSP H.265 描述失败: %w", err)
	}

	var h265 *format.H265
	h265Media := desc.FindFormat(&h265)
	if h265Media == nil {
		return errors.New("RTSP 码流已不再是 H.265")
	}
	if err := client.SetupAll(desc.BaseURL, []*description.Media{h265Media}); err != nil {
		return fmt.Errorf("建立 RTSP H.265 RTP 通道失败: %w", err)
	}

	firstPacket := make(chan struct{}, 1)
	client.OnPacketRTP(h265Media, h265, func(_ *rtp.Packet) {
		select {
		case firstPacket <- struct{}{}:
		default:
		}
	})

	if _, err := client.Play(nil); err != nil {
		return fmt.Errorf("启动 RTSP H.265 播放失败: %w", err)
	}

	waitErr := make(chan error, 1)
	go func() {
		waitErr <- client.Wait()
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-firstPacket:
		return nil
	case err := <-waitErr:
		if err == nil {
			return errors.New("RTSP H.265 原码流未收到视频包")
		}
		return fmt.Errorf("RTSP H.265 原码流中断: %w", err)
	}
}

func streamRTSPH265DirectToWebRTC(
	ctx context.Context,
	camera store.Camera,
	track *webrtc.TrackLocalStaticRTP,
	source webRTCRTSPSource,
) error {
	target, err := base.ParseURL(source.URL)
	if err != nil {
		return fmt.Errorf("RTSP 地址格式不正确: %w", err)
	}

	client := newWebRTSPClient(ctx, target, camera)
	if err := client.Start(); err != nil {
		return fmt.Errorf("连接 RTSP H.265 失败: %w", err)
	}
	defer client.Close()

	desc, _, err := client.Describe(target)
	if err != nil {
		return fmt.Errorf("读取 RTSP H.265 描述失败: %w", err)
	}

	var h265 *format.H265
	h265Media := desc.FindFormat(&h265)
	if h265Media == nil {
		return errors.New("RTSP 码流已不再是 H.265")
	}
	if err := client.SetupAll(desc.BaseURL, []*description.Media{h265Media}); err != nil {
		return fmt.Errorf("建立 RTSP H.265 RTP 通道失败: %w", err)
	}

	writeErr := make(chan error, 1)
	client.OnPacketRTP(h265Media, h265, func(pkt *rtp.Packet) {
		if err := track.WriteRTP(pkt); err != nil {
			select {
			case writeErr <- err:
			default:
			}
		}
	})

	if _, err := client.Play(nil); err != nil {
		return fmt.Errorf("启动 RTSP H.265 播放失败: %w", err)
	}

	waitErr := make(chan error, 1)
	go func() {
		waitErr <- client.Wait()
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-writeErr:
		return fmt.Errorf("发送原始 H.265 RTP 到 WebRTC 失败: %w", err)
	case err := <-waitErr:
		if err == nil {
			return errors.New("RTSP H.265 原码流已结束")
		}
		return fmt.Errorf("RTSP H.265 原码流中断: %w", err)
	}
}
