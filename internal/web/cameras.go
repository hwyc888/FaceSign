package web

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/tls"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hwyc888/FaceSign/internal/store"
)


type networkCameraFrameCache struct {
	mu        sync.Mutex
	frame     []byte
	width     int
	height    int
	fetchedAt time.Time
}

var sharedCameraTransports = struct {
	sync.Mutex
	secure   *http.Transport
	insecure *http.Transport
}{}

func cameraHTTPTransport(tlsInsecure bool) *http.Transport {
	sharedCameraTransports.Lock()
	defer sharedCameraTransports.Unlock()

	target := &sharedCameraTransports.secure
	if tlsInsecure {
		target = &sharedCameraTransports.insecure
	}
	if *target != nil {
		return *target
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 32
	transport.MaxIdleConnsPerHost = 8
	transport.IdleConnTimeout = 60 * time.Second
	if tlsInsecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	*target = transport
	return transport
}

const networkCameraPreviewFPS = 25

func networkCameraFrameInterval(camera store.Camera) time.Duration {
	fps := camera.FPS
	if fps < 1 {
		fps = 1
	}
	if fps > 12 {
		fps = 12
	}
	return time.Second / time.Duration(fps)
}

func cameraPreviewFrameInterval(camera store.Camera) time.Duration {
	fps := camera.FPS
	if camera.Kind == "network" {
		fps = networkCameraPreviewFPS
	}
	if fps < 1 {
		fps = 1
	}
	if fps > 30 {
		fps = 30
	}
	return time.Second / time.Duration(fps)
}

func (s *Server) cachedNetworkCameraFrame(ctx context.Context, camera store.Camera) ([]byte, int, int, error) {
	return s.cachedNetworkCameraFrameFor(ctx, camera, networkCameraFrameInterval(camera))
}

func (s *Server) cachedNetworkCameraFrameFor(ctx context.Context, camera store.Camera, maxAge time.Duration) ([]byte, int, int, error) {
	s.networkCameraMu.Lock()
	if s.networkCameraFrames == nil {
		s.networkCameraFrames = make(map[int64]*networkCameraFrameCache)
	}
	cache := s.networkCameraFrames[camera.ID]
	if cache == nil {
		cache = &networkCameraFrameCache{}
		s.networkCameraFrames[camera.ID] = cache
	}
	s.networkCameraMu.Unlock()

	cache.mu.Lock()
	defer cache.mu.Unlock()

	if maxAge > 0 && len(cache.frame) > 0 && time.Since(cache.fetchedAt) < maxAge {
		return cache.frame, cache.width, cache.height, nil
	}

	frame, width, height, err := fetchNetworkCameraFrame(ctx, camera)
	if err != nil {
		return nil, 0, 0, err
	}
	cache.frame = frame
	cache.width = width
	cache.height = height
	cache.fetchedAt = time.Now()
	return cache.frame, cache.width, cache.height, nil
}

func (s *Server) invalidateNetworkCameraFrame(cameraID int64) {
	s.networkCameraMu.Lock()
	delete(s.networkCameraFrames, cameraID)
	s.networkCameraMu.Unlock()
	s.stopNetworkCameraStream(cameraID)
}

type cameraRequest struct {
	CameraID      int64   `json:"camera_id,omitempty"`
	Name          string  `json:"name"`
	Kind          string  `json:"kind"`
	DeviceID      string  `json:"device_id"`
	Protocol      string  `json:"protocol"`
	StreamURL     string  `json:"stream_url"`
	SnapshotURL   string  `json:"snapshot_url"`
	Username      string  `json:"username"`
	Password      *string `json:"password"`
	ClearPassword bool    `json:"clear_password"`
	AuthMode      string  `json:"auth_mode"`
	AgentID       string  `json:"agent_id"`
	AgentSecret   *string `json:"agent_secret"`
	Width         int     `json:"width"`
	Height        int     `json:"height"`
	FPS           int     `json:"fps"`
	TimeoutMS     int     `json:"timeout_ms"`
	TLSInsecure   bool    `json:"tls_insecure"`
	IsDefault     bool    `json:"is_default"`
}


type cameraTestCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type cameraTestResult struct {
	OK            bool              `json:"ok"`
	Message       string            `json:"message"`
	ElapsedMS     int64             `json:"elapsed_ms"`
	Width         int               `json:"width,omitempty"`
	Height        int               `json:"height,omitempty"`
	DetectedAuth  string            `json:"detected_auth,omitempty"`
	PrimaryMode   string            `json:"primary_mode,omitempty"`
	FallbackUsed  bool              `json:"fallback_used,omitempty"`
	PreviewBase64 string            `json:"preview_base64,omitempty"`
	Checks        []cameraTestCheck `json:"checks"`
}

func cameraTestCheckItem(name, status, message string) cameraTestCheck {
	return cameraTestCheck{Name: name, Status: status, Message: message}
}

func cameraTestFailure(err error, elapsed time.Duration, authRequired bool, detectedAuth string) cameraTestResult {
	message := strings.TrimSpace(err.Error())
	lower := strings.ToLower(message)
	result := cameraTestResult{
		OK:        false,
		Message:   message,
		ElapsedMS:    elapsed.Milliseconds(),
		DetectedAuth: detectedAuth,
		Checks: []cameraTestCheck{
			cameraTestCheckItem("参数检查", "ok", "参数格式有效"),
		},
	}

	switch {
	case strings.Contains(lower, "401") || strings.Contains(lower, "403") ||
		strings.Contains(lower, "digest认证") || strings.Contains(lower, "digest authentication"):
		result.Message = "认证失败：请检查认证方式、用户名和密码"
		result.Checks = append(result.Checks,
			cameraTestCheckItem("网络连接", "ok", "摄像头地址可以访问"),
			cameraTestCheckItem("身份认证", "error", result.Message),
			cameraTestCheckItem("图像抓取", "pending", "认证未通过，尚未读取图像"),
		)
	case errors.Is(err, context.DeadlineExceeded) || strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "deadline exceeded"):
		result.Message = "连接超时：请检查摄像头IP、端口、网络连通性和超时时间"
		result.Checks = append(result.Checks,
			cameraTestCheckItem("网络连接", "error", result.Message),
			cameraTestCheckItem("身份认证", "pending", "尚未建立连接"),
			cameraTestCheckItem("图像抓取", "pending", "尚未建立连接"),
		)
	case strings.Contains(lower, "connection refused"):
		result.Message = "连接被拒绝：IP可能可达，但摄像头端口未开放或服务未启动"
		result.Checks = append(result.Checks,
			cameraTestCheckItem("网络连接", "error", result.Message),
			cameraTestCheckItem("身份认证", "pending", "尚未建立连接"),
			cameraTestCheckItem("图像抓取", "pending", "尚未建立连接"),
		)
	case strings.Contains(lower, "no route to host") || strings.Contains(lower, "network is unreachable") ||
		strings.Contains(lower, "host is down") || strings.Contains(lower, "no such host"):
		result.Message = "网络不可达：请检查摄像头IP、网关、VLAN/路由和DNS设置"
		result.Checks = append(result.Checks,
			cameraTestCheckItem("网络连接", "error", result.Message),
			cameraTestCheckItem("身份认证", "pending", "尚未建立连接"),
			cameraTestCheckItem("图像抓取", "pending", "尚未建立连接"),
		)
	case strings.Contains(lower, "x509") || strings.Contains(lower, "certificate"):
		result.Message = "HTTPS证书验证失败：如果摄像头使用自签名证书，可勾选“允许自签名 HTTPS 证书”后重试"
		result.Checks = append(result.Checks,
			cameraTestCheckItem("网络连接", "ok", "已连接到HTTPS服务"),
			cameraTestCheckItem("HTTPS证书", "error", result.Message),
			cameraTestCheckItem("图像抓取", "pending", "HTTPS握手未完成"),
		)
	case strings.Contains(lower, "返回的不是可识别图片") || strings.Contains(lower, "mjpeg") ||
		strings.Contains(lower, "完整jpeg帧"):
		result.Message = "已连接摄像头，但抓图地址没有返回有效图片；请检查抓图URL或MJPEG地址"
		authMessage := "无需认证"
		if authRequired {
			authMessage = "认证已通过"
		}
		result.Checks = append(result.Checks,
			cameraTestCheckItem("网络连接", "ok", "摄像头已响应"),
			cameraTestCheckItem("身份认证", "ok", authMessage),
			cameraTestCheckItem("图像抓取", "error", result.Message),
		)
	case strings.Contains(lower, "http 404"):
		result.Message = "摄像头已响应，但抓图路径不存在（HTTP 404）；请检查抓图地址"
		result.Checks = append(result.Checks,
			cameraTestCheckItem("网络连接", "ok", "摄像头已响应"),
			cameraTestCheckItem("身份认证", "ok", "连接已建立"),
			cameraTestCheckItem("图像抓取", "error", result.Message),
		)
	case strings.Contains(lower, "http "):
		result.Message = "摄像头已响应，但返回异常HTTP状态：" + message
		result.Checks = append(result.Checks,
			cameraTestCheckItem("网络连接", "ok", "摄像头已响应"),
			cameraTestCheckItem("身份认证", "ok", "连接已建立"),
			cameraTestCheckItem("图像抓取", "error", result.Message),
		)
	default:
		result.Message = "连接失败：" + message
		result.Checks = append(result.Checks,
			cameraTestCheckItem("网络连接", "error", result.Message),
			cameraTestCheckItem("身份认证", "pending", "无法确认"),
			cameraTestCheckItem("图像抓取", "pending", "无法确认"),
		)
	}
	return result
}

func cameraFromNormalizedInput(in store.CameraInput) store.Camera {
	return store.Camera{
		Name: in.Name, Kind: in.Kind, DeviceID: in.DeviceID, Protocol: in.Protocol,
		StreamURL: in.StreamURL, SnapshotURL: in.SnapshotURL, Username: in.Username,
		Password: in.Password, AuthMode: in.AuthMode, Width: in.Width, Height: in.Height,
		FPS: in.FPS, TimeoutMS: in.TimeoutMS, TLSInsecure: in.TLSInsecure, IsDefault: in.IsDefault,
	}
}

func (s *Server) cameraTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var in cameraRequest
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.ToLower(strings.TrimSpace(in.Kind)) != "network" {
		writeJSON(w, http.StatusOK, cameraTestResult{
			OK: false,
			Message: "服务器连接测试用于网络摄像头；本机摄像头请使用浏览器设备测试",
			Checks: []cameraTestCheck{cameraTestCheckItem("参数检查", "error", "请选择“网络摄像头（服务器直连）”")},
		})
		return
	}

	password := ""
	if in.CameraID > 0 {
		if current, err := s.store.CameraByID(r.Context(), in.CameraID); err == nil {
			password = current.Password
		}
	}
	if in.ClearPassword {
		password = ""
	} else if in.Password != nil {
		password = *in.Password
	}
	normalized, err := store.NormalizeCameraInput(in.storeInput(password, ""))
	if err != nil {
		writeJSON(w, http.StatusOK, cameraTestResult{
			OK: false,
			Message: "参数错误：" + err.Error(),
			Checks: []cameraTestCheck{cameraTestCheckItem("参数检查", "error", err.Error())},
		})
		return
	}

	camera := cameraFromNormalizedInput(normalized)
	started := time.Now()
	var primaryErr error
	if mode := networkCameraContinuousMode(camera); mode != "" {
		frame, width, height, source, err := s.probeNetworkCameraPrimaryFrame(r.Context(), camera)
		if err == nil {
			modeLabel := "MJPEG"
			if source == "rtsp" {
				modeLabel = "RTSP"
			}
			writeJSON(w, http.StatusOK, cameraTestResult{
				OK: true,
				Message: modeLabel + " 连续流连接成功，预览将使用连续流；人脸识别从共享帧池低帧率取样",
				ElapsedMS: time.Since(started).Milliseconds(),
				Width: width,
				Height: height,
				PrimaryMode: source,
				PreviewBase64: base64.StdEncoding.EncodeToString(frame),
				Checks: []cameraTestCheck{
					cameraTestCheckItem("参数检查", "ok", "参数格式有效"),
					cameraTestCheckItem("连续流主通道", "ok", modeLabel+" 已持续输出视频帧"),
					cameraTestCheckItem("共享帧池", "ok", fmt.Sprintf("已收到 %d×%d 实时帧；预览和识别共用同一帧池", width, height)),
					cameraTestCheckItem("HTTP抓图回退", "ok", "已保留为连续流断开时的备用通道"),
				},
			})
			return
		}
		primaryErr = err
	}

	frame, width, height, detectedAuth, err := fetchNetworkCameraFrameWithAuth(r.Context(), camera)
	elapsed := time.Since(started)
	reportedAuth := ""
	if camera.AuthMode == "auto" {
		reportedAuth = detectedAuth
	}
	if err != nil {
		authRequired := detectedAuth == "basic" || detectedAuth == "digest" || camera.AuthMode == "basic" || camera.AuthMode == "digest"
		result := cameraTestFailure(err, elapsed, authRequired, reportedAuth)
		if primaryErr != nil {
			result.Checks = append([]cameraTestCheck{
				cameraTestCheckItem("连续流主通道", "error", "连续流失败："+primaryErr.Error()),
			}, result.Checks...)
		}
		writeJSON(w, http.StatusOK, result)
		return
	}

	authMessage := "无需认证"
	switch {
	case camera.AuthMode == "auto" && detectedAuth == "digest":
		authMessage = "自动检测到 Digest，认证通过"
	case camera.AuthMode == "auto" && detectedAuth == "basic":
		authMessage = "自动检测到 Basic，认证通过"
	case camera.AuthMode == "auto":
		authMessage = "自动检测：无需认证"
	case camera.AuthMode != "none":
		authMessage = "认证通过（" + strings.ToUpper(camera.AuthMode) + "）"
	}

	result := cameraTestResult{
		OK: true,
		Message: "网络摄像头连接成功，已成功读取实时图像",
		ElapsedMS: elapsed.Milliseconds(),
		Width: width,
		Height: height,
		DetectedAuth: reportedAuth,
		PrimaryMode: "snapshot",
		PreviewBase64: base64.StdEncoding.EncodeToString(frame),
		Checks: []cameraTestCheck{
			cameraTestCheckItem("参数检查", "ok", "参数格式有效"),
			cameraTestCheckItem("网络连接", "ok", "摄像头地址可访问"),
			cameraTestCheckItem("身份认证", "ok", authMessage),
			cameraTestCheckItem("图像抓取", "ok", fmt.Sprintf("成功读取 %d×%d 图像", width, height)),
		},
	}
	if primaryErr != nil {
		result.FallbackUsed = true
		result.PrimaryMode = "snapshot-fallback"
		result.Message = "连续流当前不可用，但 HTTP Snapshot 回退成功；签到可继续使用，画面流畅度会降低"
		result.Checks = append([]cameraTestCheck{
			cameraTestCheckItem("连续流主通道", "error", "连续流失败："+primaryErr.Error()),
		}, result.Checks...)
		result.Checks = append(result.Checks,
			cameraTestCheckItem("HTTP抓图回退", "ok", "已自动切换到 HTTP Snapshot；连续流恢复后会自动切回"),
		)
	}
	writeJSON(w, http.StatusOK, result)
}

func (in cameraRequest) storeInput(password, agentSecretHash string) store.CameraInput {
	return store.CameraInput{
		Name: in.Name, Kind: in.Kind, DeviceID: in.DeviceID, Protocol: in.Protocol,
		StreamURL: in.StreamURL, SnapshotURL: in.SnapshotURL, Username: in.Username,
		Password: password, AuthMode: in.AuthMode, AgentID: in.AgentID, AgentSecretHash: agentSecretHash,
		Width: in.Width, Height: in.Height, FPS: in.FPS, TimeoutMS: in.TimeoutMS,
		TLSInsecure: in.TLSInsecure, IsDefault: in.IsDefault,
	}
}

func (s *Server) cameras(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := s.store.ListCameras(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		for i := range items { s.decorateCameraAgentState(&items[i]) }
		writeJSON(w, http.StatusOK, items)
	case http.MethodPost:
		var in cameraRequest
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		password := ""
		if in.Password != nil {
			password = *in.Password
		}
		agentSecretHash := ""
		if in.AgentSecret != nil && strings.TrimSpace(*in.AgentSecret) != "" {
			agentSecretHash = hashCameraAgentSecret(*in.AgentSecret)
		}
		item, err := s.store.CreateCamera(r.Context(), in.storeInput(password, agentSecretHash))
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		s.decorateCameraAgentState(&item)
		writeJSON(w, http.StatusCreated, item)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) cameraAction(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/cameras/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, errors.New("摄像头不存在"))
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, errors.New("摄像头ID不正确"))
		return
	}

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodPut:
			current, err := s.store.CameraByID(r.Context(), id)
			if err != nil {
				writeCameraStoreError(w, err)
				return
			}
			var in cameraRequest
			if err := decodeJSON(r, &in); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			password := current.Password
			if in.ClearPassword {
				password = ""
			} else if in.Password != nil && *in.Password != "" {
				password = *in.Password
			}
			agentSecretHash := current.AgentSecretHash
			if in.AgentSecret != nil && strings.TrimSpace(*in.AgentSecret) != "" {
				agentSecretHash = hashCameraAgentSecret(*in.AgentSecret)
			}
			item, err := s.store.UpdateCamera(r.Context(), id, in.storeInput(password, agentSecretHash))
			if err != nil {
				writeCameraStoreError(w, err)
				return
			}
			s.invalidateNetworkCameraFrame(id)
			s.decorateCameraAgentState(&item)
			writeJSON(w, http.StatusOK, item)
		case http.MethodDelete:
			if err := s.store.DeleteCamera(r.Context(), id); err != nil {
				writeCameraStoreError(w, err)
				return
			}
			s.invalidateNetworkCameraFrame(id)
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		default:
			methodNotAllowed(w)
		}
		return
	}

	if len(parts) == 2 && parts[1] == "default" && r.Method == http.MethodPost {
		item, err := s.store.SetDefaultCamera(r.Context(), id)
		if err != nil {
			writeCameraStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}

	if len(parts) == 2 && (parts[1] == "frame" || parts[1] == "stream" || parts[1] == "webrtc" || parts[1] == "recognize") {
		item, err := s.store.CameraByID(r.Context(), id)
		if err != nil {
			writeCameraStoreError(w, err)
			return
		}

		switch parts[1] {
		case "frame":
			if r.Method != http.MethodGet {
				methodNotAllowed(w)
				return
			}
			frame, width, height, status, err := s.cameraSourceFrame(r.Context(), item)
			if err != nil {
				writeError(w, status, err)
				return
			}
			w.Header().Set("Content-Type", "image/jpeg")
			w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
			w.Header().Set("X-Camera-Width", strconv.Itoa(width))
			w.Header().Set("X-Camera-Height", strconv.Itoa(height))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(frame)
			return
		case "stream":
			if r.Method != http.MethodGet {
				methodNotAllowed(w)
				return
			}
			s.cameraStream(w, r, item)
			return
		case "webrtc":
			s.cameraWebRTCOffer(w, r, item)
			return
		case "recognize":
			if r.Method != http.MethodPost {
				methodNotAllowed(w)
				return
			}
			frame, _, _, status, err := s.cameraSourceFrame(r.Context(), item)
			if err != nil {
				writeError(w, status, err)
				return
			}
			img, _, err := image.Decode(bytes.NewReader(frame))
			if err != nil {
				writeError(w, http.StatusBadGateway, fmt.Errorf("摄像头帧解码失败: %w", err))
				return
			}
			s.recognizeImage(w, r, img)
			return
		}
	}

	methodNotAllowed(w)
}

func (s *Server) cameraSourceFrame(ctx context.Context, item store.Camera) ([]byte, int, int, int, error) {
	switch item.Kind {
	case "network":
		frame, width, height, _, err := s.networkCameraFrame(ctx, item)
		if err != nil {
			return nil, 0, 0, http.StatusBadGateway, fmt.Errorf("读取网络摄像头失败: %w", err)
		}
		return frame, width, height, http.StatusOK, nil
	case "agent":
		frame, width, height, err := s.latestCameraAgentFrame(item.AgentID)
		if err != nil {
			return nil, 0, 0, http.StatusServiceUnavailable, err
		}
		return frame, width, height, http.StatusOK, nil
	default:
		return nil, 0, 0, http.StatusBadRequest, errors.New("本机摄像头画面由浏览器直接读取")
	}
}

func (s *Server) cameraPreviewSourceFrame(ctx context.Context, item store.Camera) ([]byte, int, int, int, error) {
	if item.Kind == "network" {
		frame, width, height, _, err := s.networkCameraPreviewFrame(ctx, item)
		if err != nil {
			return nil, 0, 0, http.StatusBadGateway, fmt.Errorf("读取网络摄像头失败: %w", err)
		}
		return frame, width, height, http.StatusOK, nil
	}
	return s.cameraSourceFrame(ctx, item)
}

func (s *Server) cameraStream(w http.ResponseWriter, r *http.Request, item store.Camera) {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, errors.New("当前HTTP服务不支持实时摄像头流"))
		return
	}

	frame, width, height, status, err := s.cameraPreviewSourceFrame(r.Context(), item)
	if err != nil {
		writeError(w, status, err)
		return
	}

	var continuous *networkCameraStream
	var continuousSequence uint64
	if item.Kind == "network" {
		continuous = s.ensureNetworkCameraPreviewStream(item)
		if continuous != nil {
			if pooled, ok := continuous.current(0); ok {
				continuousSequence = pooled.sequence
			}
		}
	}

	const boundary = "facesign-frame"
	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary="+boundary)
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	interval := cameraPreviewFrameInterval(item)
	consecutiveFailures := 0
	for {
		cycleStarted := time.Now()
		if _, err := fmt.Fprintf(w,
			"--%s\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\nX-Camera-Width: %d\r\nX-Camera-Height: %d\r\n\r\n",
			boundary, len(frame), width, height,
		); err != nil {
			return
		}
		if _, err := w.Write(frame); err != nil {
			return
		}
		if _, err := w.Write([]byte("\r\n")); err != nil {
			return
		}
		flusher.Flush()

		if continuous != nil {
			waitForNext := 750 * time.Millisecond
			if continuous.error() != nil {
				waitForNext = 50 * time.Millisecond
			}
			next, waitErr := continuous.waitNext(r.Context(), continuousSequence, waitForNext)
			if waitErr == nil {
				frame = next.data
				width = next.width
				height = next.height
				continuousSequence = next.sequence
				consecutiveFailures = 0
				continue
			}
			if r.Context().Err() != nil {
				return
			}
		}

		nextFrame, nextWidth, nextHeight, _, err := s.cameraPreviewSourceFrame(r.Context(), item)
		if err != nil {
			if r.Context().Err() != nil {
				return
			}
			consecutiveFailures++
			if consecutiveFailures == 1 || consecutiveFailures%10 == 0 {
				s.logger.Warn("camera preview frame retrying",
					"camera_id", item.ID,
					"camera", item.Name,
					"failures", consecutiveFailures,
					"error", err,
				)
			}
			retryDelay := 250 * time.Millisecond
			if consecutiveFailures >= 4 {
				retryDelay = 500 * time.Millisecond
			}
			if consecutiveFailures >= 10 {
				retryDelay = time.Second
			}
			timer := time.NewTimer(retryDelay)
			select {
			case <-r.Context().Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			continue
		}
		frame, width, height = nextFrame, nextWidth, nextHeight
		consecutiveFailures = 0

		delay := interval - time.Since(cycleStarted)
		if delay <= 0 {
			continue
		}
		timer := time.NewTimer(delay)
		select {
		case <-r.Context().Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func writeCameraStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, errors.New("摄像头不存在"))
		return
	}
	writeError(w, http.StatusBadRequest, err)
}

func fetchNetworkCameraFrame(ctx context.Context, camera store.Camera) ([]byte, int, int, error) {
	frame, width, height, _, err := fetchNetworkCameraFrameWithAuth(ctx, camera)
	return frame, width, height, err
}

func fetchNetworkCameraFrameWithAuth(ctx context.Context, camera store.Camera) ([]byte, int, int, string, error) {
	rawURL := camera.SnapshotURL
	mjpeg := false
	if rawURL == "" && camera.Protocol == "mjpeg" {
		rawURL = camera.StreamURL
		mjpeg = true
	}
	if rawURL == "" {
		return nil, 0, 0, "", errors.New("没有可用于识别的HTTP/HTTPS抓图地址")
	}

	client := &http.Client{
		Transport: cameraHTTPTransport(camera.TLSInsecure),
		Timeout: time.Duration(camera.TimeoutMS) * time.Millisecond,
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

	response, detectedAuth, err := doCameraRequest(ctx, client, camera, rawURL)
	if err != nil {
		return nil, 0, 0, detectedAuth, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, 0, 0, detectedAuth, fmt.Errorf("摄像头返回 HTTP %d", response.StatusCode)
	}

	const maxFrameBytes = 16 << 20
	var data []byte
	if mjpeg || strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "multipart/x-mixed-replace") {
		data, err = readFirstJPEG(response.Body, maxFrameBytes)
		if err != nil {
			return nil, 0, 0, detectedAuth, err
		}
	} else {
		data, err = io.ReadAll(io.LimitReader(response.Body, maxFrameBytes+1))
		if err != nil {
			return nil, 0, 0, detectedAuth, err
		}
		if len(data) > maxFrameBytes {
			return nil, 0, 0, detectedAuth, errors.New("摄像头图像超过16MB限制")
		}
	}

	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, detectedAuth, fmt.Errorf("摄像头返回的不是可识别图片: %w", err)
	}
	if strings.EqualFold(format, "jpeg") {
		return data, config.Width, config.Height, detectedAuth, nil
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, detectedAuth, fmt.Errorf("摄像头返回的不是可识别图片: %w", err)
	}
	frame, width, height, err := encodeJPEG(img)
	return frame, width, height, detectedAuth, err
}

func encodeJPEG(img image.Image) ([]byte, int, int, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		return nil, 0, 0, err
	}
	bounds := img.Bounds()
	return buf.Bytes(), bounds.Dx(), bounds.Dy(), nil
}

func doCameraRequest(ctx context.Context, client *http.Client, camera store.Camera, rawURL string) (*http.Response, string, error) {
	build := func(authorization string, basic bool) (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "image/jpeg,image/png,multipart/x-mixed-replace,*/*")
		req.Header.Set("User-Agent", "FaceSign/Camera")
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
		return response, "none", err
	case "basic":
		response, _, err := send("", true)
		return response, "basic", err
	case "digest":
		response, req, err := send("", false)
		if err != nil {
			return nil, "digest", err
		}
		if response.StatusCode != http.StatusUnauthorized {
			return response, "digest", nil
		}
		challenge := findCameraAuthChallenge(response.Header.Values("WWW-Authenticate"), "digest")
		_ = response.Body.Close()
		authorization, err := digestAuthorization(challenge, camera.Username, camera.Password, http.MethodGet, req.URL.RequestURI())
		if err != nil {
			return nil, "digest", err
		}
		response, _, err = send(authorization, false)
		return response, "digest", err
	case "auto":
		response, req, err := send("", false)
		if err != nil {
			return nil, "", err
		}
		if response.StatusCode != http.StatusUnauthorized && response.StatusCode != http.StatusForbidden {
			return response, "none", nil
		}

		challenges := response.Header.Values("WWW-Authenticate")
		if challenge := findCameraAuthChallenge(challenges, "digest"); challenge != "" {
			_ = response.Body.Close()
			if strings.TrimSpace(camera.Username) == "" {
				return nil, "digest", errors.New("自动检测到Digest认证，但未填写用户名")
			}
			authorization, err := digestAuthorization(challenge, camera.Username, camera.Password, http.MethodGet, req.URL.RequestURI())
			if err != nil {
				return nil, "digest", err
			}
			response, _, err = send(authorization, false)
			return response, "digest", err
		}

		if findCameraAuthChallenge(challenges, "basic") != "" || (len(challenges) == 0 && strings.TrimSpace(camera.Username) != "") {
			_ = response.Body.Close()
			if strings.TrimSpace(camera.Username) == "" {
				return nil, "basic", errors.New("自动检测到Basic认证，但未填写用户名")
			}
			response, _, err = send("", true)
			return response, "basic", err
		}
		return response, "", nil
	default:
		return nil, "", fmt.Errorf("不支持的摄像头认证方式 %s", camera.AuthMode)
	}
}

func findCameraAuthChallenge(values []string, scheme string) string {
	prefix := strings.ToLower(strings.TrimSpace(scheme)) + " "
	for _, value := range values {
		lower := strings.ToLower(value)
		index := strings.Index(lower, prefix)
		if index < 0 {
			continue
		}
		challenge := strings.TrimSpace(value[index:])
		for _, other := range []string{", basic ", ", digest "} {
			if strings.HasPrefix(other, ", "+strings.ToLower(scheme)+" ") {
				continue
			}
			if cut := strings.Index(strings.ToLower(challenge), other); cut > 0 {
				challenge = strings.TrimSpace(challenge[:cut])
			}
		}
		return challenge
	}
	return ""
}

func digestAuthorization(challenge, username, password, method, uri string) (string, error) {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(challenge)), "digest ") {
		return "", errors.New("摄像头没有返回Digest认证参数")
	}
	params := parseDigestParams(strings.TrimSpace(challenge)[7:])
	realm := params["realm"]
	nonce := params["nonce"]
	if realm == "" || nonce == "" {
		return "", errors.New("Digest认证缺少realm或nonce")
	}
	algorithm := strings.ToLower(params["algorithm"])
	if algorithm == "" {
		algorithm = "md5"
	}
	if algorithm != "md5" && algorithm != "md5-sess" {
		return "", fmt.Errorf("暂不支持Digest算法 %s", params["algorithm"])
	}
	cnonceBytes := make([]byte, 8)
	if _, err := rand.Read(cnonceBytes); err != nil {
		return "", err
	}
	cnonce := hex.EncodeToString(cnonceBytes)
	ha1 := md5Hex(username + ":" + realm + ":" + password)
	if algorithm == "md5-sess" {
		ha1 = md5Hex(ha1 + ":" + nonce + ":" + cnonce)
	}
	ha2 := md5Hex(method + ":" + uri)
	qop := ""
	for _, candidate := range strings.Split(params["qop"], ",") {
		if strings.EqualFold(strings.TrimSpace(candidate), "auth") {
			qop = "auth"
			break
		}
	}
	nc := "00000001"
	var response string
	if qop == "auth" {
		response = md5Hex(ha1 + ":" + nonce + ":" + nc + ":" + cnonce + ":" + qop + ":" + ha2)
	} else {
		response = md5Hex(ha1 + ":" + nonce + ":" + ha2)
	}
	parts := []string{
		`username="` + escapeDigest(username) + `"`,
		`realm="` + escapeDigest(realm) + `"`,
		`nonce="` + escapeDigest(nonce) + `"`,
		`uri="` + escapeDigest(uri) + `"`,
		`response="` + response + `"`,
	}
	if params["algorithm"] != "" {
		parts = append(parts, "algorithm="+params["algorithm"])
	}
	if opaque := params["opaque"]; opaque != "" {
		parts = append(parts, `opaque="`+escapeDigest(opaque)+`"`)
	}
	if qop == "auth" {
		parts = append(parts, "qop=auth", "nc="+nc, `cnonce="`+cnonce+`"`)
	}
	return "Digest " + strings.Join(parts, ", "), nil
}

func parseDigestParams(value string) map[string]string {
	out := make(map[string]string)
	start := 0
	quoted := false
	escaped := false
	parts := make([]string, 0)
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '\\':
			if quoted {
				escaped = !escaped
			}
		case '"':
			if !escaped {
				quoted = !quoted
			}
			escaped = false
		case ',':
			if !quoted {
				parts = append(parts, value[start:i])
				start = i + 1
			}
			escaped = false
		default:
			escaped = false
		}
	}
	parts = append(parts, value[start:])
	for _, part := range parts {
		pair := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(pair) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(pair[0]))
		val := strings.TrimSpace(pair[1])
		if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
			val = val[1 : len(val)-1]
		}
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
	return strings.ReplaceAll(value, `"`, `\\"`)
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
					if previous == 0xff && current == 0xd8 {
						started = true
						out = append(out, 0xff, 0xd8)
					}
				} else {
					out = append(out, current)
					if previous == 0xff && current == 0xd9 {
						return out, nil
					}
				}
				previous = current
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
	}
	return nil, errors.New("MJPEG数据中没有读取到完整JPEG帧")
}
