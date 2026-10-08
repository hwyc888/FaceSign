package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/hwyc888/FaceSign/internal/store"
)

type hostCameraDevice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func dshowQuotedValue(line string) string {
	start := strings.IndexByte(line, '"')
	if start < 0 {
		return ""
	}
	rest := line[start+1:]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:end])
}

func parseDShowVideoDevices(output string) []hostCameraDevice {
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	devices := make([]hostCameraDevice, 0, 4)
	videoSection := false
	last := -1
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if closeBracket := strings.IndexByte(line, ']'); closeBracket >= 0 {
			line = strings.TrimSpace(line[closeBracket+1:])
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "directshow video devices") {
			videoSection = true
			last = -1
			continue
		}
		if strings.Contains(lower, "directshow audio devices") {
			videoSection = false
			last = -1
			continue
		}
		if !videoSection {
			continue
		}
		if strings.HasPrefix(lower, "alternative name") {
			if last >= 0 {
				if id := dshowQuotedValue(line); id != "" {
					devices[last].ID = id
				}
			}
			continue
		}
		if !strings.HasPrefix(line, "\"") {
			continue
		}
		// FFmpeg output differs by build/version: some builds append "(video)"
		// after the friendly name, while others print only the quoted name.
		// The section heading already identifies these as video devices.
		name := dshowQuotedValue(line)
		if name == "" {
			continue
		}
		devices = append(devices, hostCameraDevice{ID: name, Name: name})
		last = len(devices) - 1
	}
	seen := make(map[string]bool, len(devices))
	out := devices[:0]
	for _, device := range devices {
		key := strings.ToLower(device.ID)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, device)
	}
	return out
}

func listDShowCameraDevices(ctx context.Context, ffmpegPath string) ([]hostCameraDevice, string, error) {
	var lastOutput string
	var lastErr error
	for _, input := range []string{"dummy", "0"} {
		cmd := exec.CommandContext(ctx, ffmpegPath,
			"-hide_banner",
			"-list_devices", "true",
			"-f", "dshow",
			"-i", input,
		)
		output, err := cmd.CombinedOutput()
		lastOutput = string(output)
		lastErr = err
		if devices := parseDShowVideoDevices(lastOutput); len(devices) > 0 {
			return devices, lastOutput, nil
		}
		if ctx.Err() != nil {
			return nil, lastOutput, ctx.Err()
		}
	}
	return nil, lastOutput, lastErr
}

func parsePnPCameraLines(output string) []hostCameraDevice {
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	devices := make([]hostCameraDevice, 0, 4)
	seen := map[string]bool{}
	for _, line := range lines {
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		instanceID := strings.TrimSpace(parts[1])
		if name == "" {
			continue
		}
		key := strings.ToLower(name + "\x00" + instanceID)
		if seen[key] {
			continue
		}
		seen[key] = true
		devices = append(devices, hostCameraDevice{ID: name, Name: name})
	}
	return devices
}

func listWindowsPnPCameras(ctx context.Context) ([]hostCameraDevice, string, error) {
	if runtime.GOOS != "windows" {
		return nil, "", errors.New("PnP camera enumeration is only available on Windows")
	}
	script := "$items = Get-PnpDevice -PresentOnly -ErrorAction SilentlyContinue | " +
		"Where-Object { ($_.Class -eq 'Camera' -or $_.Class -eq 'Image') -and $_.Status -eq 'OK' }; " +
		"$items | ForEach-Object { [Console]::WriteLine([string]::Concat($_.FriendlyName,[char]9,$_.InstanceId)) }"
	cmd := exec.CommandContext(ctx,
		"powershell.exe",
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy", "Bypass",
		"-Command", script,
	)
	output, err := cmd.CombinedOutput()
	text := string(output)
	return parsePnPCameraLines(text), text, err
}

func deviceNames(devices []hostCameraDevice) string {
	names := make([]string, 0, len(devices))
	for _, device := range devices {
		name := strings.TrimSpace(device.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	return strings.Join(names, "、")
}

func hostCameraDiagnosticChecks(ctx context.Context) []cameraTestCheck {
	checks := make([]cameraTestCheck, 0, 2)
	ffmpegPath, ffmpegErr := findFFmpeg()
	if ffmpegErr != nil {
		checks = append(checks, cameraTestCheckItem("FFmpeg DirectShow", "error", "未找到可用 FFmpeg："+ffmpegErr.Error()))
	} else {
		dshowDevices, _, dshowErr := listDShowCameraDevices(ctx, ffmpegPath)
		switch {
		case len(dshowDevices) > 0:
			checks = append(checks, cameraTestCheckItem("FFmpeg DirectShow", "ok", "已检测到："+deviceNames(dshowDevices)))
		case dshowErr != nil:
			checks = append(checks, cameraTestCheckItem("FFmpeg DirectShow", "error", "未枚举到视频设备："+dshowErr.Error()))
		default:
			checks = append(checks, cameraTestCheckItem("FFmpeg DirectShow", "error", "命令已执行，但没有返回视频设备"))
		}
	}

	pnpDevices, _, pnpErr := listWindowsPnPCameras(ctx)
	switch {
	case len(pnpDevices) > 0:
		checks = append(checks, cameraTestCheckItem("Windows PnP", "ok", "Windows硬件层已检测到："+deviceNames(pnpDevices)))
	case pnpErr != nil:
		checks = append(checks, cameraTestCheckItem("Windows PnP", "error", "无法读取Windows摄像头设备："+pnpErr.Error()))
	default:
		checks = append(checks, cameraTestCheckItem("Windows PnP", "error", "Windows也没有检测到 Camera/Image 类摄像头"))
	}
	return checks
}

func listHostCameraDevices(ctx context.Context) ([]hostCameraDevice, error) {
	if runtime.GOOS != "windows" {
		return nil, errors.New("FaceSign主机USB摄像头当前仅支持Windows")
	}
	ffmpegPath, err := findFFmpeg()
	if err != nil {
		return nil, err
	}

	dshowDevices, dshowOutput, dshowErr := listDShowCameraDevices(ctx, ffmpegPath)
	if len(dshowDevices) > 0 {
		return dshowDevices, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	pnpDevices, _, pnpErr := listWindowsPnPCameras(ctx)
	if len(pnpDevices) > 0 {
		return pnpDevices, nil
	}

	lowerOutput := strings.ToLower(dshowOutput)
	switch {
	case pnpErr == nil && strings.Contains(lowerOutput, "directshow video devices"):
		return nil, errors.New("FFmpeg进入了DirectShow设备枚举，但Windows PnP和FFmpeg都没有返回摄像头；请先在设备管理器确认摄像头处于正常状态")
	case pnpErr == nil:
		return nil, errors.New("Windows PnP和FFmpeg DirectShow都未检测到摄像头；请检查USB连接、摄像头驱动和Windows摄像头权限")
	case dshowErr != nil:
		return nil, fmt.Errorf("无法枚举FaceSign主机USB摄像头：DirectShow=%v；Windows PnP=%v", dshowErr, pnpErr)
	default:
		return nil, fmt.Errorf("无法枚举FaceSign主机USB摄像头：Windows PnP=%v", pnpErr)
	}
}

func resolveHostCameraDevice(ctx context.Context, configured string) (hostCameraDevice, bool, error) {
	devices, err := listHostCameraDevices(ctx)
	if err != nil {
		return hostCameraDevice{}, false, err
	}
	configured = strings.TrimSpace(configured)
	if configured != "" {
		for _, device := range devices {
			if strings.EqualFold(configured, device.ID) || strings.EqualFold(configured, device.Name) {
				return device, false, nil
			}
		}
	}
	return devices[0], configured != "", nil
}

func hostCameraFFmpegArgs(camera store.Camera, deviceID string) []string {
	width := camera.Width
	if width <= 0 {
		width = 1280
	}
	height := camera.Height
	if height <= 0 {
		height = 720
	}
	fps := camera.FPS
	if fps <= 0 {
		fps = 30
	}
	if fps > 30 {
		fps = 30
	}
	filter := fmt.Sprintf("fps=%d,scale=%d:%d:force_original_aspect_ratio=decrease:force_divisible_by=2", fps, width, height)
	return []string{
		"-hide_banner",
		"-loglevel", "error",
		"-nostdin",
		"-f", "dshow",
		"-thread_queue_size", "64",
		"-rtbufsize", "128M",
		"-i", "video=" + deviceID,
		"-map", "0:v:0",
		"-an",
		"-sn",
		"-dn",
		"-vf", filter,
		"-c:v", "mjpeg",
		"-q:v", "7",
		"-flush_packets", "1",
		"-f", "image2pipe",
		"pipe:1",
	}
}

func (s *Server) consumeHostCameraStream(ctx context.Context, stream *networkCameraStream) error {
	ffmpegPath, err := findFFmpeg()
	if err != nil {
		return err
	}
	deviceCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	device, fellBack, err := resolveHostCameraDevice(deviceCtx, stream.camera.DeviceID)
	cancel()
	if err != nil {
		return err
	}
	if fellBack {
		s.logger.Warn("saved local camera id is not a host DirectShow device; using first host USB camera",
			"camera_id", stream.camera.ID,
			"camera", stream.camera.Name,
			"configured_device_id", stream.camera.DeviceID,
			"host_device", device.Name,
		)
	}

	command := exec.CommandContext(ctx, ffmpegPath, hostCameraFFmpegArgs(stream.camera, device.ID)...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	command.WaitDelay = 2 * time.Second
	stdout, err := command.StdoutPipe()
	if err != nil {
		return fmt.Errorf("创建主机USB摄像头输出管道失败: %w", err)
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("启动FaceSign主机USB摄像头失败: %w", err)
	}

	readErr := readJPEGSequence(ctx, stdout, func(frame []byte) error {
		return stream.publish(frame, "usb")
	})
	if readErr != nil && ctx.Err() == nil && command.Process != nil {
		_ = command.Process.Kill()
	}
	waitErr := command.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return fmt.Errorf("读取FaceSign主机USB摄像头失败: %w", readErr)
	}
	if waitErr != nil {
		detail := strings.TrimSpace(stderr.String())
		if len(detail) > 500 {
			detail = detail[len(detail)-500:]
		}
		if detail == "" {
			detail = waitErr.Error()
		}
		return fmt.Errorf("FaceSign主机USB摄像头已停止: %s", detail)
	}
	return errors.New("FaceSign主机USB摄像头视频流已结束")
}

func (s *Server) hostCameraDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	devices, err := listHostCameraDevices(ctx)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	writeJSON(w, http.StatusOK, devices)
}
