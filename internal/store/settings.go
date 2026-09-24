package store

import (
	"context"
	"database/sql"
	"errors"
)

const (
	autoStartCheckinSettingKey    = "auto_start_checkin"
	realtimeStatusEnabledSettingKey = "realtime_status_enabled"
)

type AppSettings struct {
	AutoStartCheckin      bool `json:"auto_start_checkin"`
	RealtimeStatusEnabled bool `json:"realtime_status_enabled"`
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
	return AppSettings{
		AutoStartCheckin:      autoStart,
		RealtimeStatusEnabled: realtimeStatus,
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
