package store

import (
	"context"
	"database/sql"
	"errors"
)

const (
	autoStartCheckinSettingKey          = "auto_start_checkin"
	realtimeStatusEnabledSettingKey     = "realtime_status_enabled"
	realtimeStatusModeSettingKey        = "realtime_status_mode"
	realtimeStatusVideoSettingKey       = "realtime_status_video"
	realtimeStatusDropSettingKey        = "realtime_status_drop"
	realtimeStatusNetworkSettingKey     = "realtime_status_network"
	realtimeStatusRecognitionSettingKey = "realtime_status_recognition"
	realtimeStatusReasonSettingKey      = "realtime_status_reason"
)

type AppSettings struct {
	AutoStartCheckin           bool `json:"auto_start_checkin"`
	RealtimeStatusEnabled      bool `json:"realtime_status_enabled"`
	RealtimeStatusMode         bool `json:"realtime_status_mode"`
	RealtimeStatusVideo        bool `json:"realtime_status_video"`
	RealtimeStatusDrop         bool `json:"realtime_status_drop"`
	RealtimeStatusNetwork      bool `json:"realtime_status_network"`
	RealtimeStatusRecognition  bool `json:"realtime_status_recognition"`
	RealtimeStatusReason       bool `json:"realtime_status_reason"`
}

func (s *Store) AppSettings(ctx context.Context) (AppSettings, error) {
	autoStart, err := s.appSettingBool(ctx, autoStartCheckinSettingKey, false)
	if err != nil {
		return AppSettings{}, err
	}
	realtimeStatus, err := s.appSettingBool(ctx, realtimeStatusEnabledSettingKey, true)
	if err != nil {
		return AppSettings{}, err
	}
	mode, err := s.appSettingBool(ctx, realtimeStatusModeSettingKey, true)
	if err != nil {
		return AppSettings{}, err
	}
	video, err := s.appSettingBool(ctx, realtimeStatusVideoSettingKey, true)
	if err != nil {
		return AppSettings{}, err
	}
	drop, err := s.appSettingBool(ctx, realtimeStatusDropSettingKey, true)
	if err != nil {
		return AppSettings{}, err
	}
	network, err := s.appSettingBool(ctx, realtimeStatusNetworkSettingKey, true)
	if err != nil {
		return AppSettings{}, err
	}
	recognition, err := s.appSettingBool(ctx, realtimeStatusRecognitionSettingKey, true)
	if err != nil {
		return AppSettings{}, err
	}
	reason, err := s.appSettingBool(ctx, realtimeStatusReasonSettingKey, true)
	if err != nil {
		return AppSettings{}, err
	}
	return AppSettings{
		AutoStartCheckin:          autoStart,
		RealtimeStatusEnabled:     realtimeStatus,
		RealtimeStatusMode:        mode,
		RealtimeStatusVideo:       video,
		RealtimeStatusDrop:        drop,
		RealtimeStatusNetwork:     network,
		RealtimeStatusRecognition: recognition,
		RealtimeStatusReason:      reason,
	}, nil
}

func (s *Store) appSettingBool(ctx context.Context, key string, defaultValue bool) (bool, error) {
	var value string
	err := s.db.QueryRowContext(ctx,
		"SELECT value FROM app_settings WHERE key=?",
		key,
	).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultValue, nil
	}
	if err != nil {
		return false, err
	}
	return value == "1", nil
}

func (s *Store) UpdateAppSettings(ctx context.Context, settings AppSettings) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	values := []struct {
		key     string
		enabled bool
	}{
		{autoStartCheckinSettingKey, settings.AutoStartCheckin},
		{realtimeStatusEnabledSettingKey, settings.RealtimeStatusEnabled},
		{realtimeStatusModeSettingKey, settings.RealtimeStatusMode},
		{realtimeStatusVideoSettingKey, settings.RealtimeStatusVideo},
		{realtimeStatusDropSettingKey, settings.RealtimeStatusDrop},
		{realtimeStatusNetworkSettingKey, settings.RealtimeStatusNetwork},
		{realtimeStatusRecognitionSettingKey, settings.RealtimeStatusRecognition},
		{realtimeStatusReasonSettingKey, settings.RealtimeStatusReason},
	}
	for _, item := range values {
		value := "0"
		if item.enabled {
			value = "1"
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO app_settings(key,value) VALUES(?,?)
			ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
			item.key, value,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}
