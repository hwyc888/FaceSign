package web

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/hwyc888/FaceSign/internal/store"
)

type resolvedRTSPSource struct {
	URL       string
	Transport string
	Codec     string
	Label     string
}

type rtspCandidate struct {
	URL   string
	Label string
}

func resolveRTSPSource(ctx context.Context, camera store.Camera, preferH264 bool) (resolvedRTSPSource, error) {
	ffmpegPath, err := findFFmpeg()
	if err != nil {
		return resolvedRTSPSource{}, err
	}
	candidates, err := rtspCandidates(camera)
	if err != nil {
		return resolvedRTSPSource{}, err
	}

	resolveTimeout := networkCameraProbeTimeout(camera)
	if preferH264 {
		resolveTimeout = cameraWebRTCProbeTimeout(camera) + 4*time.Second
		if resolveTimeout > 12*time.Second {
			resolveTimeout = 12 * time.Second
		}
	}
	resolveCtx, cancel := context.WithTimeout(ctx, resolveTimeout)
	defer cancel()

	transports := []string{"tcp", "udp"}
	var diagnostics []string
	var firstHEVC *resolvedRTSPSource
candidateLoop:
	for _, candidate := range candidates {
		for _, transport := range transports {
			if resolveCtx.Err() != nil {
				break
			}
			codec, detail, probeErr := probeRTSPCandidate(resolveCtx, ffmpegPath, camera, candidate.URL, transport)
			if probeErr != nil {
				if strings.Contains(detail, "认证失败") {
					if algorithm := inspectRTSPDigestAlgorithm(resolveCtx, candidate.URL); strings.Contains(strings.ToUpper(algorithm), "SHA") {
						return resolvedRTSPSource{}, fmt.Errorf("摄像头 RTSP Digest 算法为 %s，而当前 FFmpeg RTSP 认证仅兼容 MD5；请在海康“配置 → 系统 → 安全管理/认证”中把 RTSP Digest 算法改为 MD5 后保存", algorithm)
					}
				}
				if detail != "" {
					diagnostics = append(diagnostics, candidate.Label+"/"+strings.ToUpper(transport)+": "+detail)
				}
				continue
			}

			switch codec {
			case "h264":
				return resolvedRTSPSource{
					URL: candidate.URL, Transport: transport, Codec: codec, Label: candidate.Label,
				}, nil
			case "hevc", "h265":
				source := resolvedRTSPSource{
					URL: candidate.URL, Transport: transport, Codec: "hevc", Label: candidate.Label,
				}
				if !preferH264 {
					return source, nil
				}
				if firstHEVC == nil {
					firstHEVC = &source
				}
				diagnostics = append(diagnostics, candidate.Label+"/"+strings.ToUpper(transport)+": 检测到 H.265/HEVC")
				continue candidateLoop
			default:
				if !preferH264 {
					return resolvedRTSPSource{
						URL: candidate.URL, Transport: transport, Codec: codec, Label: candidate.Label,
					}, nil
				}
				diagnostics = append(diagnostics, candidate.Label+"/"+strings.ToUpper(transport)+": 未检测到 H.264")
				continue candidateLoop
			}
		}
	}

	if preferH264 && firstHEVC != nil {
		return *firstHEVC, nil
	}
	if len(diagnostics) == 0 {
		if resolveCtx.Err() != nil {
			return resolvedRTSPSource{}, fmt.Errorf("RTSP 连接超时：%w", resolveCtx.Err())
		}
		return resolvedRTSPSource{}, errors.New("RTSP 连接失败，未收到摄像头响应")
	}
	return resolvedRTSPSource{}, errors.New(bestRTSPDiagnostic(diagnostics))
}

func rtspCandidates(camera store.Camera) ([]rtspCandidate, error) {
	raw := strings.TrimSpace(camera.StreamURL)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return nil, errors.New("RTSP 视频流地址格式不正确")
	}

	type rawCandidate struct {
		url   string
		label string
	}
	rawCandidates := []rawCandidate{{url: raw, label: "当前地址"}}
	lowerPath := strings.ToLower(parsed.Path)
	isHikvision := strings.Contains(lowerPath, "/streaming/channels/") ||
		strings.Contains(lowerPath, "/isapi/streaming/channels/") ||
		strings.Contains(lowerPath, "/ch1/")

	if isHikvision {
		hosts := []string{parsed.Host}
		port := parsed.Port()
		hostName := parsed.Hostname()
		switch port {
		case "", "554":
			hosts = append(hosts, net.JoinHostPort(hostName, "10554"))
		case "10554":
			hosts = append(hosts, net.JoinHostPort(hostName, "554"))
		}

		paths := []struct {
			path  string
			label string
		}{
			{"/Streaming/channels/101", "海康主码流101"},
			{"/Streaming/channels/102", "海康子码流102"},
			{"/ISAPI/Streaming/Channels/101", "海康ISAPI主码流101"},
			{"/ISAPI/Streaming/Channels/102", "海康ISAPI子码流102"},
			{"/ch1/main/av_stream", "海康兼容主码流"},
			{"/ch1/sub/av_stream", "海康兼容子码流"},
		}
		for hostIndex, host := range hosts {
			for _, item := range paths {
				clone := *parsed
				clone.Host = host
				clone.Path = item.path
				clone.RawPath = ""
				clone.RawQuery = ""
				clone.Fragment = ""
				label := item.label
				if hostIndex > 0 {
					label += "@" + clone.Port()
				}
				rawCandidates = append(rawCandidates, rawCandidate{url: clone.String(), label: label})
			}
		}
	}

	seen := make(map[string]bool)
	out := make([]rtspCandidate, 0, len(rawCandidates))
	for _, item := range rawCandidates {
		input, err := rtspURLWithCameraCredentials(camera, item.url)
		if err != nil {
			continue
		}
		if seen[input] {
			continue
		}
		seen[input] = true
		out = append(out, rtspCandidate{URL: input, Label: item.label})
	}
	if len(out) == 0 {
		return nil, errors.New("没有可用的 RTSP 地址")
	}
	return out, nil
}

func rtspURLWithCameraCredentials(camera store.Camera, raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return "", errors.New("RTSP 视频流地址格式不正确")
	}
	if strings.TrimSpace(camera.Username) != "" {
		parsed.User = url.UserPassword(camera.Username, camera.Password)
	}
	return parsed.String(), nil
}

func probeRTSPCandidate(ctx context.Context, ffmpegPath string, camera store.Camera, inputURL, transport string) (string, string, error) {
	timeout := time.Duration(camera.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	if timeout > 1800*time.Millisecond {
		timeout = 1800 * time.Millisecond
	}
	attemptCtx, cancel := context.WithTimeout(ctx, timeout+500*time.Millisecond)
	defer cancel()

	args := []string{
		"-hide_banner",
		"-loglevel", "info",
		"-nostdin",
		"-rtsp_transport", transport,
		"-timeout", strconv.FormatInt(timeout.Microseconds(), 10),
		"-i", inputURL,
		"-map", "0:v:0",
		"-frames:v", "1",
		"-an",
		"-sn",
		"-dn",
		"-f", "null",
		"-",
	}
	command := exec.CommandContext(attemptCtx, ffmpegPath, args...)
	command.Stdout = io.Discard
	var stderr bytes.Buffer
	command.Stderr = &stderr
	runErr := command.Run()
	output := stderr.String()
	codec := detectFFmpegVideoCodec(output)
	if runErr == nil && codec != "" {
		return codec, "", nil
	}
	if runErr == nil {
		return "", "RTSP 已连接但 FFmpeg 未识别出视频编码", errors.New("video codec not detected")
	}
	return codec, classifyRTSPProbeFailure(output, runErr, camera, inputURL), runErr
}

func inspectRTSPDigestAlgorithm(ctx context.Context, inputURL string) string {
	parsed, err := url.Parse(inputURL)
	if err != nil || parsed.Hostname() == "" || !strings.EqualFold(parsed.Scheme, "rtsp") {
		return ""
	}
	port := parsed.Port()
	if port == "" {
		port = "554"
	}
	host := net.JoinHostPort(parsed.Hostname(), port)
	dialer := net.Dialer{Timeout: 1500 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "tcp", host)
	if err != nil {
		return ""
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	clean := *parsed
	clean.User = nil
	requestURI := clean.String()
	_, err = fmt.Fprintf(conn,
		"DESCRIBE %s RTSP/1.0\r\nCSeq: 1\r\nAccept: application/sdp\r\nUser-Agent: FaceSign\r\n\r\n",
		requestURI,
	)
	if err != nil {
		return ""
	}

	reader := bufio.NewReader(conn)
	status, err := reader.ReadString('\n')
	if err != nil || !strings.Contains(status, "401") {
		return ""
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return ""
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return ""
		}
		if !strings.HasPrefix(strings.ToLower(line), "www-authenticate:") {
			continue
		}
		value := strings.TrimSpace(line[len("www-authenticate:"):])
		if !strings.HasPrefix(strings.ToLower(value), "digest ") {
			continue
		}
		params := parseDigestParams(strings.TrimSpace(value[7:]))
		algorithm := strings.TrimSpace(params["algorithm"])
		if algorithm == "" {
			return "MD5"
		}
		return algorithm
	}
}

func detectFFmpegVideoCodec(output string) string {
	lower := strings.ToLower(output)
	switch {
	case strings.Contains(lower, "video: h264"):
		return "h264"
	case strings.Contains(lower, "video: hevc"), strings.Contains(lower, "video: h265"):
		return "hevc"
	case strings.Contains(lower, "video: mjpeg"):
		return "mjpeg"
	default:
		return ""
	}
}

func classifyRTSPProbeFailure(output string, runErr error, camera store.Camera, inputURL string) string {
	lower := strings.ToLower(output)
	switch {
	case strings.Contains(lower, "401 unauthorized"), strings.Contains(lower, "unauthorized"):
		return "RTSP 认证失败（401），请检查用户名、密码或摄像头 RTSP 认证方式"
	case strings.Contains(lower, "404 not found"), strings.Contains(lower, "method describe failed: 404"):
		return "RTSP 路径不存在（404）"
	case strings.Contains(lower, "connection refused"):
		return "RTSP 端口拒绝连接，请检查摄像头 RTSP 端口是否已启用"
	case strings.Contains(lower, "connection timed out"), strings.Contains(lower, "timed out"), strings.Contains(lower, "i/o error"):
		return "RTSP 连接超时，请检查 RTSP 端口、网络或摄像头服务"
	case strings.Contains(lower, "no route to host"), strings.Contains(lower, "network is unreachable"):
		return "无法到达摄像头 RTSP 网络"
	case strings.Contains(lower, "server returned 4"), strings.Contains(lower, "method describe failed"):
		return "摄像头拒绝 RTSP DESCRIBE 请求"
	}

	detail := lastFFmpegDiagnosticLine(output)
	if detail == "" {
		detail = runErr.Error()
	}
	detail = sanitizeRTSPDiagnostic(detail, camera, inputURL)
	if len(detail) > 180 {
		detail = detail[:180] + "…"
	}
	return "FFmpeg：" + detail
}

func sanitizeRTSPDiagnostic(text string, camera store.Camera, inputURL string) string {
	out := strings.TrimSpace(text)
	if camera.Password != "" {
		out = strings.ReplaceAll(out, camera.Password, "***")
	}
	if inputURL != "" {
		out = strings.ReplaceAll(out, inputURL, redactRTSPURL(inputURL))
	}
	return out
}

func redactRTSPURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if parsed.User != nil {
		parsed.User = url.User("***")
	}
	return parsed.String()
}

func lastFFmpegDiagnosticLine(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		return line
	}
	return ""
}

func bestRTSPDiagnostic(items []string) string {
	for _, item := range items {
		lower := strings.ToLower(item)
		if strings.Contains(lower, "认证失败") || strings.Contains(lower, "401") {
			return item
		}
	}
	for _, item := range items {
		lower := strings.ToLower(item)
		if strings.Contains(lower, "端口拒绝") || strings.Contains(lower, "超时") || strings.Contains(lower, "路径不存在") {
			return item
		}
	}
	return items[0]
}
