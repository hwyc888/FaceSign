package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hwyc888/FaceSign/internal/store"
)

type hikvisionH264OptimizationResult struct {
	OK       bool   `json:"ok"`
	Changed  bool   `json:"changed"`
	Verified bool   `json:"verified"`
	Codec    string `json:"codec"`
	FPS      int    `json:"fps"`
	GOP      int    `json:"gop"`
	Message  string `json:"message"`
}

func optimizeHikvisionH264(ctx context.Context, camera store.Camera) (hikvisionH264OptimizationResult, error) {
	if camera.Kind != "network" || strings.ToLower(strings.TrimSpace(camera.Protocol)) != "rtsp" || !isHikvisionCamera(camera) {
		return hikvisionH264OptimizationResult{}, errors.New("只有海康 RTSP 网络摄像头支持此 H.264 自动优化")
	}

	endpoint, err := hikvisionMainStreamConfigURL(camera)
	if err != nil {
		return hikvisionH264OptimizationResult{}, err
	}
	current, err := hikvisionReadStreamConfig(ctx, camera, endpoint)
	if err != nil {
		return hikvisionH264OptimizationResult{}, fmt.Errorf("读取海康 101 主码流参数失败: %w", err)
	}

	updated, fps, gop, changed, err := optimizeHikvisionStreamXML(current)
	if err != nil {
		return hikvisionH264OptimizationResult{}, err
	}
	if !changed {
		return hikvisionH264OptimizationResult{
			OK: true, Changed: false, Verified: true,
			Codec: "H.264", FPS: fps, GOP: gop,
			Message: fmt.Sprintf("海康 101 主码流已经是 H.264 / %d FPS / GOP %d，无需重复修改", fps, gop),
		}, nil
	}

	if err := hikvisionWriteStreamConfig(ctx, camera, endpoint, updated); err != nil {
		return hikvisionH264OptimizationResult{}, fmt.Errorf("写入海康 101 主码流参数失败: %w", err)
	}

	select {
	case <-ctx.Done():
		return hikvisionH264OptimizationResult{}, ctx.Err()
	case <-time.After(250 * time.Millisecond):
	}

	result := hikvisionH264OptimizationResult{
		OK: true, Changed: true,
		Codec: "H.264", FPS: fps, GOP: gop,
		Message: fmt.Sprintf("海康 101 主码流已优化为 H.264 / %d FPS / GOP %d；FaceSign 将优先使用 H.264 WebRTC 直通", fps, gop),
	}
	verified, verifyErr := hikvisionReadStreamConfig(ctx, camera, endpoint)
	if verifyErr == nil {
		codec := strings.TrimSpace(hikvisionXMLValue(verified, "videoCodecType"))
		actualFPS := hikvisionFrameRateToFPS(hikvisionXMLInt(verified, "maxFrameRate"))
		actualGOP := hikvisionXMLInt(verified, "GovLength")
		if strings.EqualFold(strings.ReplaceAll(codec, ".", ""), "H264") {
			result.Verified = true
			if actualFPS > 0 {
				result.FPS = actualFPS
			}
			if actualGOP > 0 {
				result.GOP = actualGOP
			}
		}
	}
	return result, nil
}

func isHikvisionCamera(camera store.Camera) bool {
	combined := strings.ToLower(camera.StreamURL + "\n" + camera.SnapshotURL)
	return strings.Contains(combined, "/streaming/channels/") ||
		strings.Contains(combined, "/isapi/streaming/channels/")
}

func hikvisionMainStreamConfigURL(camera store.Camera) (string, error) {
	for _, raw := range []string{camera.SnapshotURL, camera.StreamURL} {
		u, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || u.Hostname() == "" {
			continue
		}
		if u.Scheme == "http" || u.Scheme == "https" {
			return u.Scheme + "://" + u.Host + "/ISAPI/Streaming/channels/101", nil
		}
		if u.Scheme == "rtsp" || u.Scheme == "rtsps" {
			host := u.Hostname()
			if strings.Contains(host, ":") {
				host = "[" + host + "]"
			}
			return "http://" + host + "/ISAPI/Streaming/channels/101", nil
		}
	}
	return "", errors.New("无法从摄像头地址确定海康 ISAPI 管理地址")
}

func optimizeHikvisionStreamXML(current []byte) ([]byte, int, int, bool, error) {
	codec := strings.TrimSpace(hikvisionXMLValue(current, "videoCodecType"))
	if codec == "" {
		return nil, 0, 0, false, errors.New("海康返回的主码流配置缺少 videoCodecType")
	}
	currentRate := hikvisionXMLInt(current, "maxFrameRate")
	if currentRate <= 0 {
		return nil, 0, 0, false, errors.New("海康返回的主码流配置缺少 maxFrameRate")
	}
	if hikvisionXMLValue(current, "GovLength") == "" {
		return nil, 0, 0, false, errors.New("海康返回的主码流配置缺少 GovLength")
	}

	fps := hikvisionFrameRateToFPS(currentRate)
	if fps <= 0 {
		fps = 25
	}
	if fps > 25 {
		fps = 25
	}
	rateUnit := 1
	if currentRate > 100 {
		rateUnit = 100
	}
	targetRate := fps * rateUnit
	gop := fps

	updated := append([]byte(nil), current...)
	var ok bool
	updated, ok = replaceHikvisionXMLValue(updated, "videoCodecType", "H.264")
	if !ok {
		return nil, 0, 0, false, errors.New("无法修改海康 videoCodecType")
	}
	updated, ok = replaceHikvisionXMLValue(updated, "maxFrameRate", strconv.Itoa(targetRate))
	if !ok {
		return nil, 0, 0, false, errors.New("无法修改海康 maxFrameRate")
	}
	updated, ok = replaceHikvisionXMLValue(updated, "GovLength", strconv.Itoa(gop))
	if !ok {
		return nil, 0, 0, false, errors.New("无法修改海康 GovLength")
	}

	changed := !bytes.Equal(bytes.TrimSpace(updated), bytes.TrimSpace(current))
	return updated, fps, gop, changed, nil
}

func hikvisionFrameRateToFPS(rate int) int {
	if rate > 100 {
		return rate / 100
	}
	return rate
}

func hikvisionXMLValue(doc []byte, name string) string {
	pattern := `(?is)<(?:[A-Za-z0-9_.-]+:)?` + regexp.QuoteMeta(name) + `(?:\s[^>]*)?>\s*([^<]*?)\s*</(?:[A-Za-z0-9_.-]+:)?` + regexp.QuoteMeta(name) + `\s*>`
	match := regexp.MustCompile(pattern).FindSubmatch(doc)
	if len(match) != 2 {
		return ""
	}
	return strings.TrimSpace(string(match[1]))
}

func hikvisionXMLInt(doc []byte, name string) int {
	value, _ := strconv.Atoi(hikvisionXMLValue(doc, name))
	return value
}

func replaceHikvisionXMLValue(doc []byte, name, value string) ([]byte, bool) {
	pattern := `(?is)(<(?:[A-Za-z0-9_.-]+:)?` + regexp.QuoteMeta(name) + `(?:\s[^>]*)?>)[^<]*(</(?:[A-Za-z0-9_.-]+:)?` + regexp.QuoteMeta(name) + `\s*>)`
	re := regexp.MustCompile(pattern)
	if !re.Match(doc) {
		return doc, false
	}
	return re.ReplaceAll(doc, []byte("${1}"+value+"${2}")), true
}

func hikvisionReadStreamConfig(ctx context.Context, camera store.Camera, endpoint string) ([]byte, error) {
	response, err := doCameraControlRequest(ctx, camera, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("摄像头返回 HTTP %d", response.StatusCode)
	}
	return io.ReadAll(io.LimitReader(response.Body, 1<<20))
}

func hikvisionWriteStreamConfig(ctx context.Context, camera store.Camera, endpoint string, payload []byte) error {
	response, err := doCameraControlRequest(ctx, camera, http.MethodPut, endpoint, payload)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("摄像头返回 HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	if statusCode := hikvisionXMLInt(body, "statusCode"); statusCode > 1 {
		return fmt.Errorf("海康拒绝修改，statusCode=%d", statusCode)
	}
	return nil
}

func doCameraControlRequest(ctx context.Context, camera store.Camera, method, rawURL string, payload []byte) (*http.Response, error) {
	timeout := time.Duration(camera.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	client := &http.Client{Transport: cameraHTTPTransport(camera.TLSInsecure), Timeout: timeout}

	build := func(authorization string, basic bool) (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, method, rawURL, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/xml,text/xml,*/*")
		req.Header.Set("User-Agent", "FaceSign/Camera")
		if len(payload) > 0 {
			req.Header.Set("Content-Type", "application/xml")
		}
		if basic {
			req.SetBasicAuth(camera.Username, camera.Password)
		}
		if authorization != "" {
			req.Header.Set("Authorization", authorization)
		}
		return req, nil
	}
	send := func(authorization string, basic bool) (*http.Response, *http.Request, error) {
		req, err := build(authorization, basic)
		if err != nil {
			return nil, nil, err
		}
		response, err := client.Do(req)
		return response, req, err
	}

	switch camera.AuthMode {
	case "none":
		response, _, err := send("", false)
		return response, err
	case "basic":
		response, _, err := send("", true)
		return response, err
	case "digest":
		response, req, err := send("", false)
		if err != nil {
			return nil, err
		}
		if response.StatusCode != http.StatusUnauthorized {
			return response, nil
		}
		challenge := findCameraAuthChallenge(response.Header.Values("WWW-Authenticate"), "digest")
		_ = response.Body.Close()
		authorization, err := digestAuthorization(challenge, camera.Username, camera.Password, method, req.URL.RequestURI())
		if err != nil {
			return nil, err
		}
		response, _, err = send(authorization, false)
		return response, err
	case "auto":
		response, req, err := send("", false)
		if err != nil {
			return nil, err
		}
		if response.StatusCode != http.StatusUnauthorized && response.StatusCode != http.StatusForbidden {
			return response, nil
		}
		challenges := response.Header.Values("WWW-Authenticate")
		if challenge := findCameraAuthChallenge(challenges, "digest"); challenge != "" {
			_ = response.Body.Close()
			authorization, err := digestAuthorization(challenge, camera.Username, camera.Password, method, req.URL.RequestURI())
			if err != nil {
				return nil, err
			}
			response, _, err = send(authorization, false)
			return response, err
		}
		if findCameraAuthChallenge(challenges, "basic") != "" || (len(challenges) == 0 && strings.TrimSpace(camera.Username) != "") {
			_ = response.Body.Close()
			response, _, err = send("", true)
			return response, err
		}
		return response, nil
	default:
		return nil, fmt.Errorf("不支持的摄像头认证方式 %s", camera.AuthMode)
	}
}
