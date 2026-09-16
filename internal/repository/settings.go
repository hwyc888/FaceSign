package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hwyc888/FaceSign/internal/domain"
)

const faceSettingsKey = "face.settings"

func (r *Repository) LoadFaceSettings(ctx context.Context, defaults domain.FaceSettings) (domain.FaceSettings, error) {
	var raw string
	err := r.db.QueryRowContext(ctx, `SELECT value FROM system_settings WHERE key=?`, faceSettingsKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return defaults, nil
	}
	if err != nil {
		return domain.FaceSettings{}, err
	}
	settings := defaults
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return domain.FaceSettings{}, fmt.Errorf("decode face settings: %w", err)
	}
	return settings, nil
}

func (r *Repository) SaveFaceSettings(ctx context.Context, settings domain.FaceSettings) error {
	raw, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("encode face settings: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO system_settings(key,value,updated_at) VALUES(?,?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at`,
		faceSettingsKey, string(raw), time.Now().Unix())
	return err
}
