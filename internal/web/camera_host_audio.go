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

		if !strings.HasPrefix(line, "\"") {
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
	if len(devices) == 0 { return nil, errors.New("FaceSign主机未检测到可用的DirectShow音频输入设备；请确认摄像头麦克风已在Windows声音输入设备中启用，并允许桌面应用访问麦克风") }
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

type windowsPnPDeviceContainer struct {
	Name        string
	InstanceID  string
	Class       string
	ContainerID string
}

func parseWindowsPnPDeviceContainers(output string) []windowsPnPDeviceContainer {
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	devices := make([]windowsPnPDeviceContainer, 0, 8)
	seen := map[string]bool{}
	for _, line := range lines {
		parts := strings.Split(line, "\t")
		if len(parts) < 4 {
			continue
		}
		device := windowsPnPDeviceContainer{
			Name:        strings.TrimSpace(parts[0]),
			InstanceID:  strings.TrimSpace(parts[1]),
			Class:       strings.TrimSpace(parts[2]),
			ContainerID: strings.TrimSpace(parts[3]),
		}
		if device.Name == "" || device.ContainerID == "" {
			continue
		}
		key := strings.ToLower(device.InstanceID + "\x00" + device.ContainerID)
		if seen[key] {
			continue
		}
		seen[key] = true
		devices = append(devices, device)
	}
	return devices
}

func listWindowsPnPDeviceContainers(ctx context.Context) ([]windowsPnPDeviceContainer, error) {
	if runtime.GOOS != "windows" {
		return nil, errors.New("Windows PnP设备关联仅支持Windows")
	}
	script := "$items = Get-PnpDevice -PresentOnly -ErrorAction SilentlyContinue | " +
		"Where-Object { $_.Status -eq 'OK' -and ($_.Class -eq 'Camera' -or $_.Class -eq 'Image' -or $_.Class -eq 'AudioEndpoint' -or $_.Class -eq 'Media') }; " +
		"$items | ForEach-Object { " +
		"$property = Get-PnpDeviceProperty -InstanceId $_.InstanceId -KeyName 'DEVPKEY_Device_ContainerId' -ErrorAction SilentlyContinue; " +
		"if ($null -ne $property -and $null -ne $property.Data) { " +
		"[Console]::WriteLine([string]::Concat($_.FriendlyName,[char]9,$_.InstanceId,[char]9,$_.Class,[char]9,[string]$property.Data)) } }"
	cmd := exec.CommandContext(ctx,
		"powershell.exe",
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy", "Bypass",
		"-Command", script,
	)
	output, err := cmd.CombinedOutput()
	devices := parseWindowsPnPDeviceContainers(string(output))
	if len(devices) > 0 {
		return devices, nil
	}
	if err != nil {
		return nil, err
	}
	return nil, errors.New("Windows PnP未返回可用于摄像头和麦克风关联的设备容器")
}

func normalizedHostDeviceName(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(name)), " "))
}

func matchHostAudioDeviceByPnPContainer(videoName string, audioDevices []hostCameraDevice, pnpDevices []windowsPnPDeviceContainer) (hostCameraDevice, bool) {
	videoKey := normalizedHostDeviceName(videoName)
	if videoKey == "" {
		return hostCameraDevice{}, false
	}

	videoContainer := ""
	for _, device := range pnpDevices {
		if !strings.EqualFold(device.Class, "Camera") && !strings.EqualFold(device.Class, "Image") {
			continue
		}
		if normalizedHostDeviceName(device.Name) != videoKey {
			continue
		}
		if videoContainer == "" {
			videoContainer = device.ContainerID
			continue
		}
		if !strings.EqualFold(videoContainer, device.ContainerID) {
			return hostCameraDevice{}, false
		}
	}
	if videoContainer == "" {
		return hostCameraDevice{}, false
	}

	pairedPnP := make([]windowsPnPDeviceContainer, 0, 4)
	for _, device := range pnpDevices {
		if !strings.EqualFold(device.ContainerID, videoContainer) {
			continue
		}
		if strings.EqualFold(device.Class, "AudioEndpoint") || strings.EqualFold(device.Class, "Media") {
			pairedPnP = append(pairedPnP, device)
		}
	}
	if len(pairedPnP) == 0 {
		return hostCameraDevice{}, false
	}

	for _, audio := range audioDevices {
		audioKey := normalizedHostDeviceName(audio.Name)
		for _, paired := range pairedPnP {
			if audioKey != "" && audioKey == normalizedHostDeviceName(paired.Name) {
				return audio, true
			}
	}

	best := hostCameraDevice{}
	bestScore := 0
	tied := false
	for _, audio := range audioDevices {
		score := 0
		for _, paired := range pairedPnP {
			if candidate := hostAudioMatchScore(paired.Name, audio.Name); candidate > score {
				score = candidate
			}
		}
		if score > bestScore {
			best = audio
			bestScore = score
			tied = false
		} else if score > 0 && score == bestScore && !strings.EqualFold(best.ID, audio.ID) {
			tied = true
		}
	}
	if bestScore <= 0 || tied {
		return hostCameraDevice{}, false
	}
	return best, true
}

func selectHostAudioDevice(ctx context.Context, camera store.Camera) (hostCameraDevice, error) {
	devices, err := listHostAudioDevices(ctx)
	if err != nil {
		return hostCameraDevice{}, err
	}

	video, _, err := resolveHostCameraDevice(ctx, camera.DeviceID)
	if err != nil {
		return hostCameraDevice{}, errors.New("无法确认当前USB摄像头，不能安全匹配它自己的麦克风")
	}
	videoName := strings.TrimSpace(video.Name)
	if videoName == "" {
		return hostCameraDevice{}, errors.New("当前USB摄像头没有可用于匹配麦克风的设备名称")
	}

	if pnpDevices, pnpErr := listWindowsPnPDeviceContainers(ctx); pnpErr == nil {
		if matched, ok := matchHostAudioDeviceByPnPContainer(videoName, devices, pnpDevices); ok {
			return matched, nil
		}
	}

	best := hostCameraDevice{}
	bestScore := 0
	for _, device := range devices {
		score := hostAudioMatchScore(videoName, device.Name)
		if score > bestScore {
			best = device
			bestScore = score
		}
	}
	if bestScore <= 0 {
		names := deviceNames(devices)
		if names != "" {
			return hostCameraDevice{}, errors.New("已检测到麦克风输入（" + names + "），但无法确认哪一个与当前USB摄像头属于同一设备；请确认Windows“设置 > 隐私和安全性 > 麦克风 > 允许桌面应用访问麦克风”已开启")
		}
		return hostCameraDevice{}, errors.New("没有找到与当前USB摄像头匹配的麦克风；请确认Windows麦克风权限已允许桌面应用访问")
	}
	return best, nil
}
