package cameraio

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"strings"
	"time"
)

type Config struct {
	Protocol string `json:"protocol"`
	StreamURL string `json:"stream_url"`
	SnapshotURL string `json:"snapshot_url"`
	Username string `json:"username"`
	Password string `json:"password"`
	AuthMode string `json:"auth_mode"`
	TimeoutMS int `json:"timeout_ms"`
	TLSInsecure bool `json:"tls_insecure"`
}

func FetchFrame(ctx context.Context, cfg Config) ([]byte, int, int, error) {
	if cfg.TimeoutMS == 0 { cfg.TimeoutMS = 3000 }
	rawURL := strings.TrimSpace(cfg.SnapshotURL)
	mjpeg := false
	if rawURL == "" && strings.EqualFold(cfg.Protocol, "mjpeg") { rawURL = strings.TrimSpace(cfg.StreamURL); mjpeg = true }
	if rawURL == "" { return nil, 0, 0, errors.New("没有可用于识别的HTTP/HTTPS抓图地址") }
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.TLSInsecure { transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} }
	client := &http.Client{
		Transport: transport,
		Timeout: time.Duration(cfg.TimeoutMS) * time.Millisecond,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 { return errors.New("摄像头重定向次数过多") }
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" { return errors.New("摄像头重定向到了不支持的协议") }
			return nil
		},
	}
	response, err := doRequest(ctx, client, cfg, rawURL)
	if err != nil { return nil, 0, 0, err }
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 { return nil, 0, 0, fmt.Errorf("摄像头返回 HTTP %d", response.StatusCode) }
	var data []byte
	if mjpeg || strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "multipart/x-mixed-replace") {
		data, err = readFirstJPEG(response.Body, 16<<20)
		if err != nil { return nil, 0, 0, err }
	} else {
		img, _, err := image.Decode(io.LimitReader(response.Body, 16<<20))
		if err != nil { return nil, 0, 0, fmt.Errorf("摄像头返回的不是可识别图片: %w", err) }
		return encodeJPEG(img)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil { return nil, 0, 0, fmt.Errorf("MJPEG帧解码失败: %w", err) }
	return encodeJPEG(img)
}

func encodeJPEG(img image.Image) ([]byte, int, int, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil { return nil, 0, 0, err }
	b := img.Bounds()
	return buf.Bytes(), b.Dx(), b.Dy(), nil
}

func doRequest(ctx context.Context, client *http.Client, cfg Config, rawURL string) (*http.Response, error) {
	build := func(authorization string) (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil { return nil, err }
		req.Header.Set("Accept", "image/jpeg,image/png,multipart/x-mixed-replace,*/*")
		req.Header.Set("User-Agent", "FaceSign-CameraAgent/1")
		if strings.EqualFold(cfg.AuthMode, "basic") { req.SetBasicAuth(cfg.Username, cfg.Password) }
		if authorization != "" { req.Header.Set("Authorization", authorization) }
		return req, nil
	}
	req, err := build("")
	if err != nil { return nil, err }
	response, err := client.Do(req)
	if err != nil { return nil, err }
	if !strings.EqualFold(cfg.AuthMode, "digest") || response.StatusCode != http.StatusUnauthorized { return response, nil }
	challenge := response.Header.Get("WWW-Authenticate")
	_ = response.Body.Close()
	authorization, err := digestAuthorization(challenge, cfg.Username, cfg.Password, http.MethodGet, req.URL.RequestURI())
	if err != nil { return nil, err }
	req, err = build(authorization)
	if err != nil { return nil, err }
	return client.Do(req)
}

func digestAuthorization(challenge, username, password, method, uri string) (string, error) {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(challenge)), "digest ") { return "", errors.New("摄像头没有返回Digest认证参数") }
	params := parseDigestParams(strings.TrimSpace(challenge)[7:])
	realm, nonce := params["realm"], params["nonce"]
	if realm == "" || nonce == "" { return "", errors.New("Digest认证缺少realm或nonce") }
	algorithm := strings.ToLower(params["algorithm"])
	if algorithm == "" { algorithm = "md5" }
	if algorithm != "md5" && algorithm != "md5-sess" { return "", fmt.Errorf("暂不支持Digest算法 %s", params["algorithm"]) }
	cnonceBytes := make([]byte, 8)
	if _, err := rand.Read(cnonceBytes); err != nil { return "", err }
	cnonce := hex.EncodeToString(cnonceBytes)
	ha1 := md5Hex(username + ":" + realm + ":" + password)
	if algorithm == "md5-sess" { ha1 = md5Hex(ha1 + ":" + nonce + ":" + cnonce) }
	ha2 := md5Hex(method + ":" + uri)
	qop := ""
	for _, candidate := range strings.Split(params["qop"], ",") {
		if strings.EqualFold(strings.TrimSpace(candidate), "auth") { qop = "auth"; break }
	}
	nc := "00000001"
	var response string
	if qop == "auth" { response = md5Hex(ha1 + ":" + nonce + ":" + nc + ":" + cnonce + ":" + qop + ":" + ha2) } else { response = md5Hex(ha1 + ":" + nonce + ":" + ha2) }
	parts := []string{
		"username=\"" + escapeDigest(username) + "\"",
		"realm=\"" + escapeDigest(realm) + "\"",
		"nonce=\"" + escapeDigest(nonce) + "\"",
		"uri=\"" + escapeDigest(uri) + "\"",
		"response=\"" + response + "\"",
	}
	if params["algorithm"] != "" { parts = append(parts, "algorithm="+params["algorithm"]) }
	if opaque := params["opaque"]; opaque != "" { parts = append(parts, "opaque=\""+escapeDigest(opaque)+"\"") }
	if qop == "auth" { parts = append(parts, "qop=auth", "nc="+nc, "cnonce=\""+cnonce+"\"") }
	return "Digest " + strings.Join(parts, ", "), nil
}

func parseDigestParams(value string) map[string]string {
	out := make(map[string]string)
	start := 0
	quoted, escaped := false, false
	parts := make([]string, 0)
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '\\':
			if quoted { escaped = !escaped }
		case '"':
			if !escaped { quoted = !quoted }
			escaped = false
		case ',':
			if !quoted { parts = append(parts, value[start:i]); start = i + 1 }
			escaped = false
		default:
			escaped = false
		}
	}
	parts = append(parts, value[start:])
	for _, part := range parts {
		pair := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(pair) != 2 { continue }
		key := strings.ToLower(strings.TrimSpace(pair[0]))
		val := strings.TrimSpace(pair[1])
		if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' { val = val[1:len(val)-1] }
		out[key] = val
	}
	return out
}

func md5Hex(value string) string {
	sum := md5.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}

func escapeDigest(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	return strings.ReplaceAll(value, "\"", "\\\"")
}

func readFirstJPEG(reader io.Reader, limit int) ([]byte, error) {
	buf := make([]byte, 32*1024)
	out := make([]byte, 0, 512*1024)
	started := false
	var previous byte
	total := 0
	for total < limit {
		n, err := reader.Read(buf)
		if n > 0 {
			total += n
			for _, current := range buf[:n] {
				if !started {
					if previous == 0xff && current == 0xd8 { started = true; out = append(out, 0xff, 0xd8) }
				} else {
					out = append(out, current)
					if previous == 0xff && current == 0xd9 { return out, nil }
				}
				previous = current
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) { break }
			return nil, err
		}
	}
	return nil, errors.New("MJPEG数据中没有读取到完整JPEG帧")
}
