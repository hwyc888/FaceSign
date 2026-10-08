package web

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"strings"

	"github.com/hwyc888/FaceSign/internal/store"
)

func parseDShowAudioDevices(output string) []hostCameraDevice {
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	devices := make([]hostCameraDevice, 0, 4)
	audioSection := false
	last := -1
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if closeBracket := strings.IndexByte(line, ']'); closeBracket >= 0 {
			line = strings.TrimSpace(line[closeBracket+1:])
		}
		lower := strings.ToLower(line)

		// Older/custom FFmpeg builds can print section headings, while current
		// FFmpeg prints each DirectShow friendly name followed by "(audio)" or
		// "(video)". Support both forms.
		if strings.Contains(lower, "directshow audio devices") {
			audioSection = true
			last = -1
			continue
		}
		if strings.Contains(lower, "directshow video devices") {
			audioSection = false
			last = -1
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

		if !strings.HasPrefix(line, """) {
			continue
		}
		isAudioLine := audioSection || strings.Contains(lower, "(audio")
		isVideoLine := strings.Contains(lower, "(video")
		if !isAudioLine || isVideoLine {
			last = -1
			continue
		}

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

func listHostAudioDevices(ctx context.Context) ([]hostCameraDevice, error) {
	if runtime.GOOS != "windows" { return nil, errors.New("FaceSign主机音频当前仅支持Windows") }
	ffmpegPath, err := findFFmpeg()
	if err != nil { return nil, err }
	cmd := exec.CommandContext(ctx, ffmpegPath, "-hide_banner", "-list_devices", "true", "-f", "dshow", "-i", "dummy")
	output, _ := cmd.CombinedOutput()
	devices := parseDShowAudioDevices(string(output))
	if len(devices) == 0 { return nil, errors.New("FaceSign主机未检测到可用的DirectShow音频输入设备") }
	return devices, nil
}

func hostAudioMatchTokens(name string) map[string]bool {
	ignore := map[string]bool{"audio": true, "camera": true, "device": true, "hd": true, "integrated": true, "mic": true, "microphone": true, "video": true, "webcam": true}
	out := map[string]bool{}
	for _, token := range strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	}) {
		if len(token) >= 3 && !ignore[token] { out[token] = true }
	}
	return out
}

func hostAudioMatchScore(videoName, audioName string) int {
	videoTokens := hostAudioMatchTokens(videoName)
	audioTokens := hostAudioMatchTokens(audioName)
	score := 0
	for token := range videoTokens { if audioTokens[token] { score += len(token) } }
	if strings.Contains(strings.ToLower(videoName), "usb") && strings.Contains(strings.ToLower(audioName), "usb") { score += 2 }
	return score
}

func selectHostAudioDevice(ctx context.Context, camera store.Camera) (hostCameraDevice, error) {
	devices, err := listHostAudioDevices(ctx)
	if err != nil { return hostCameraDevice{}, err }
	if len(devices) == 1 { return devices[0], nil }
	videoName := ""
	if video, _, resolveErr := resolveHostCameraDevice(ctx, camera.DeviceID); resolveErr == nil { videoName = video.Name }
	best := devices[0]
	bestScore := hostAudioMatchScore(videoName, best.Name)
	for _, device := range devices[1:] {
		if score := hostAudioMatchScore(videoName, device.Name); score > bestScore { best = device; bestScore = score }
	}
	return best, nil
}
