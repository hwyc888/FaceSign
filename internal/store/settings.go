package store

import (
	"context"
	"database/sql"
	"errors"
)

const autoStartCheckinSettingKey = "auto_start_checkin"

type AppSettings struct {
	AutoStartCheckin bool `json:"auto_start_checkin"`
}

func (s *Store) AppSettings(ctx context.Context) (AppSettings, error) {
	var value string
	err := s.db.QueryRowContext(ctx,
		"SELECT value FROM app_settings WHERE key=?",
		autoStartCheckinSettingKey,
	).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return AppSettings{}, nil
	}
	if err != nil {
		return AppSettings{}, err
	}
	return AppSettings{AutoStartCheckin: value == "1"}, nil
}

func (s *Store) UpdateAppSettings(ctx context.Context, settings AppSettings) error {
	value := "0"
	if settings.AutoStartCheckin {
		value = "1"
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO app_settings(key,value) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		autoStartCheckinSettingKey, value,
	)
	return err
}
