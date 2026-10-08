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

func listHostCameraDevices(ctx context.Context) ([]hostCameraDevice, error) {
	if runtime.GOOS != "windows" {
		return nil, errors.New("FaceSign主机USB摄像头当前仅支持Windows")
	}
	ffmpegPath, err := findFFmpeg()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, ffmpegPath,
		"-hide_banner",
		"-list_devices", "true",
		"-f", "dshow",
		"-i", "dummy",
	)
	output, runErr := cmd.CombinedOutput()
	devices := parseDShowVideoDevices(string(output))
	if len(devices) > 0 {
		return devices, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if runErr != nil {
		lowerOutput := strings.ToLower(string(output))
		if strings.Contains(lowerOutput, "directshow video devices") {
			return nil, errors.New("FFmpeg 已进入 DirectShow 视频设备枚举，但没有返回任何摄像头条目；请关闭可能占用摄像头的软件，并检查 Windows 摄像头驱动与“允许桌面应用访问摄像头”权限")
		}
		return nil, errors.New("FaceSign电脑无法枚举DirectShow摄像头；请确认USB摄像头已连接、Windows隐私设置允许桌面应用访问摄像头，并用新版 FaceSignManager.exe 重新执行“安装/注册本目录”")
	}
	return nil, errors.New("FaceSign电脑未检测到可用的USB摄像头；请检查摄像头驱动和Windows摄像头权限")
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
