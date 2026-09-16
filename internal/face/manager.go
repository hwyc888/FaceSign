package face

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/hwyc888/FaceSign/internal/domain"
)

type Manager struct {
	mu       sync.RWMutex
	settings domain.FaceSettings
	provider Provider
}

func NewManager(settings domain.FaceSettings) (*Manager, error) {
	m := &Manager{}
	if err := m.Configure(settings); err != nil {
		return nil, err
	}
	return m, nil
}

func ValidateSettings(settings domain.FaceSettings) (domain.FaceSettings, error) {
	settings.Provider = strings.ToLower(strings.TrimSpace(settings.Provider))
	settings.ServiceURL = strings.TrimRight(strings.TrimSpace(settings.ServiceURL), "/")
	settings.APIKey = strings.TrimSpace(settings.APIKey)
	if settings.Provider == "" {
		settings.Provider = "disabled"
	}
	if settings.Provider != "disabled" && settings.Provider != "compreface" {
		return domain.FaceSettings{}, fmt.Errorf("不支持的人脸识别引擎 %q", settings.Provider)
	}
	if settings.Similarity < 0 || settings.Similarity > 1 {
		return domain.FaceSettings{}, fmt.Errorf("人脸相似度阈值必须在 0 到 1 之间")
	}
	if settings.DetectionThreshold < 0 || settings.DetectionThreshold > 1 {
		return domain.FaceSettings{}, fmt.Errorf("人脸检测阈值必须在 0 到 1 之间")
	}
	if settings.Provider == "compreface" {
		if settings.ServiceURL == "" {
			return domain.FaceSettings{}, fmt.Errorf("CompreFace 服务地址不能为空")
		}
		parsed, err := url.Parse(settings.ServiceURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return domain.FaceSettings{}, fmt.Errorf("CompreFace 服务地址必须是有效的 HTTP/HTTPS 地址")
		}
	}
	return settings, nil
}

func (m *Manager) Configure(settings domain.FaceSettings) error {
	normalized, err := ValidateSettings(settings)
	if err != nil {
		return err
	}
	var provider Provider = Disabled{}
	if normalized.Provider == "compreface" {
		provider = NewCompreFace(normalized.ServiceURL, normalized.APIKey, normalized.Similarity, normalized.DetectionThreshold)
	}
	m.mu.Lock()
	m.settings = normalized
	m.provider = provider
	m.mu.Unlock()
	return nil
}

func (m *Manager) Settings() domain.FaceSettings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.settings
}

func (m *Manager) snapshotProvider() Provider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.provider
}

func (m *Manager) Name() string {
	return m.snapshotProvider().Name()
}

func (m *Manager) Enabled() bool {
	return m.snapshotProvider().Enabled()
}

func (m *Manager) Check(ctx context.Context) error {
	return m.snapshotProvider().Check(ctx)
}

func (m *Manager) Enroll(ctx context.Context, subject string, image []byte) (string, error) {
	return m.snapshotProvider().Enroll(ctx, subject, image)
}

func (m *Manager) Recognize(ctx context.Context, image []byte) (Match, error) {
	return m.snapshotProvider().Recognize(ctx, image)
}

func (m *Manager) Status(ctx context.Context) domain.FaceServiceStatus {
	settings := m.Settings()
	status := domain.FaceServiceStatus{
		Provider:   settings.Provider,
		Configured: settings.Provider == "compreface" && settings.APIKey != "",
		Enabled:    m.Enabled(),
		CheckedAt:  time.Now().Unix(),
	}
	if settings.Provider == "disabled" {
		status.Message = "人脸识别服务尚未启用"
		return status
	}
	if settings.APIKey == "" {
		status.Message = "CompreFace 已选择，但尚未配置 API Key"
		return status
	}

	checkCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	if err := m.Check(checkCtx); err != nil {
		switch {
		case errors.Is(err, ErrInvalidAPIKey):
			status.Message = "CompreFace 已连接，但 API Key 无效"
		case errors.Is(err, context.DeadlineExceeded):
			status.Message = "连接 CompreFace 超时，请检查服务是否启动"
		default:
			status.Message = "无法连接 CompreFace，请检查服务地址和运行状态"
		}
		status.Detail = err.Error()
		return status
	}
	status.Reachable = true
	status.Message = "CompreFace 服务正常，API Key 验证通过"
	return status
}
