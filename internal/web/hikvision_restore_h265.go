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
	"strings"
	"time"

	"github.com/hwyc888/FaceSign/internal/store"
)

const pillar2CameraName = "大门主校道立柱2"

type hikvisionH265RestoreResult struct {
	OK bool `json:"ok"`
	Changed bool `json:"changed"`
	Verified bool `json:"verified"`
	Codec string `json:"codec"`
	Message string `json:"message"`
}

func restoreHikvisionPillar2H265(ctx context.Context, camera store.Camera) (hikvisionH265RestoreResult, error) {
	if camera.Name != pillar2CameraName {
		return hikvisionH265RestoreResult{}, fmt.Errorf("只允许恢复“大门主校道立柱2”，当前摄像头为 %q", camera.Name)
	}
	if camera.Kind != "network" || strings.ToLower(strings.TrimSpace(camera.Protocol)) != "rtsp" {
		return hikvisionH265RestoreResult{}, errors.New("立柱2不是RTSP网络摄像头")
	}
	endpoint, err := hikvisionPillar2MainStreamURL(camera)
	if err != nil { return hikvisionH265RestoreResult{}, err }
	current, err := hikvisionPillar2Request(ctx, camera, http.MethodGet, endpoint, nil)
	if err != nil { return hikvisionH265RestoreResult{}, fmt.Errorf("读取海康101主码流参数失败: %w", err) }
	codec := hikvisionPillar2XMLValue(current, "videoCodecType")
	if codec == "" { return hikvisionH265RestoreResult{}, errors.New("海康101主码流配置缺少 videoCodecType") }
	if isH265Codec(codec) {
		return hikvisionH265RestoreResult{OK:true, Changed:false, Verified:true, Codec:"H.265", Message:"大门主校道立柱2的101主码流已经是 H.265，无需修改"}, nil
	}
	updated, ok := replaceHikvisionPillar2XMLValue(current, "videoCodecType", "H.265")
	if !ok || bytes.Equal(current, updated) { return hikvisionH265RestoreResult{}, errors.New("无法只修改 videoCodecType") }
	if _, err := hikvisionPillar2Request(ctx, camera, http.MethodPut, endpoint, updated); err != nil {
		return hikvisionH265RestoreResult{}, fmt.Errorf("写入海康101主码流H.265失败: %w", err)
	}
	select { case <-ctx.Done(): return hikvisionH265RestoreResult{}, ctx.Err(); case <-time.After(300*time.Millisecond): }
	verifiedXML, err := hikvisionPillar2Request(ctx, camera, http.MethodGet, endpoint, nil)
	if err != nil { return hikvisionH265RestoreResult{}, fmt.Errorf("H.265已写入，但回读验证失败: %w", err) }
	verifiedCodec := hikvisionPillar2XMLValue(verifiedXML, "videoCodecType")
	if !isH265Codec(verifiedCodec) { return hikvisionH265RestoreResult{}, fmt.Errorf("设备未保持H.265，回读编码为 %q", verifiedCodec) }
	return hikvisionH265RestoreResult{OK:true, Changed:true, Verified:true, Codec:"H.265", Message:"大门主校道立柱2的101主码流已恢复为 H.265；FPS、GOP、分辨率、码率和FaceSign数据库均未修改"}, nil
}

func isH265Codec(value string) bool {
	n := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), ".", ""))
	return n == "h265" || n == "hevc"
}

func hikvisionPillar2MainStreamURL(camera store.Camera) (string, error) {
	for _, raw := range []string{camera.SnapshotURL, camera.StreamURL} {
		u, err := url.Parse(strings.TrimSpace(raw)); if err != nil || u.Hostname() == "" { continue }
		if u.Scheme == "http" || u.Scheme == "https" { return u.Scheme + "://" + u.Host + "/ISAPI/Streaming/channels/101", nil }
		if u.Scheme == "rtsp" || u.Scheme == "rtsps" {
			host := u.Hostname(); if strings.Contains(host, ":") { host = "[" + host + "]" }
			return "http://" + host + "/ISAPI/Streaming/channels/101", nil
		}
	}
	return "", errors.New("无法从立柱2配置确定海康ISAPI地址")
}

func hikvisionPillar2XMLValue(doc []byte, name string) string {
	pattern := "(?is)<(?:[A-Za-z0-9_.-]+:)?" + regexp.QuoteMeta(name) + "(?:\\s[^>]*)?>\\s*([^<]*?)\\s*</(?:[A-Za-z0-9_.-]+:)?" + regexp.QuoteMeta(name) + "\\s*>"
	m := regexp.MustCompile(pattern).FindSubmatch(doc)
	if len(m) != 2 { return "" }
	return strings.TrimSpace(string(m[1]))
}

func replaceHikvisionPillar2XMLValue(doc []byte, name, value string) ([]byte, bool) {
	pattern := "(?is)(<(?:[A-Za-z0-9_.-]+:)?" + regexp.QuoteMeta(name) + "(?:\\s[^>]*)?>)[^<]*(</(?:[A-Za-z0-9_.-]+:)?" + regexp.QuoteMeta(name) + "\\s*>)"
	re := regexp.MustCompile(pattern)
	if !re.Match(doc) { return doc, false }
	return re.ReplaceAll(doc, []byte("${1}"+value+"${2}")), true
}

func hikvisionPillar2Request(ctx context.Context, camera store.Camera, method, rawURL string, payload []byte) ([]byte, error) {
	timeout := time.Duration(camera.TimeoutMS)*time.Millisecond; if timeout <= 0 { timeout = 3*time.Second }
	client := &http.Client{Transport: cameraHTTPTransport(camera.TLSInsecure), Timeout: timeout}
	build := func(auth string, basic bool) (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, method, rawURL, bytes.NewReader(payload)); if err != nil { return nil, err }
		req.Header.Set("Accept", "application/xml,text/xml,*/*"); req.Header.Set("User-Agent", "FaceSign/Camera-Recovery")
		if len(payload) > 0 { req.Header.Set("Content-Type", "application/xml") }
		if basic { req.SetBasicAuth(camera.Username, camera.Password) }; if auth != "" { req.Header.Set("Authorization", auth) }
		return req, nil
	}
	send := func(auth string, basic bool) (*http.Response, *http.Request, error) { req, err := build(auth,basic); if err != nil { return nil,nil,err }; resp, err := client.Do(req); return resp,req,err }
	var resp *http.Response; var err error
	switch camera.AuthMode {
	case "none": resp,_,err = send("",false)
	case "basic": resp,_,err = send("",true)
	case "digest":
		var req *http.Request; resp,req,err = send("",false)
		if err == nil && resp.StatusCode == http.StatusUnauthorized {
			challenge := findCameraAuthChallenge(resp.Header.Values("WWW-Authenticate"), "digest"); _ = resp.Body.Close()
			var authorization string; authorization,err = digestAuthorization(challenge,camera.Username,camera.Password,method,req.URL.RequestURI())
			if err == nil { resp,_,err = send(authorization,false) }
		}
	case "auto":
		var req *http.Request; resp,req,err = send("",false)
		if err == nil && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) {
			challenges := resp.Header.Values("WWW-Authenticate")
			if challenge := findCameraAuthChallenge(challenges,"digest"); challenge != "" {
				_ = resp.Body.Close(); var authorization string; authorization,err = digestAuthorization(challenge,camera.Username,camera.Password,method,req.URL.RequestURI()); if err == nil { resp,_,err = send(authorization,false) }
			} else { _ = resp.Body.Close(); resp,_,err = send("",true) }
		}
	default: return nil, fmt.Errorf("不支持的摄像头认证方式 %s", camera.AuthMode)
	}
	if err != nil { return nil, err }; if resp == nil { return nil, errors.New("摄像头没有返回响应") }
	defer resp.Body.Close(); body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20)); if readErr != nil { return nil, readErr }
	if resp.StatusCode < 200 || resp.StatusCode >= 300 { return nil, fmt.Errorf("摄像头返回 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body))) }
	return body,nil
}
