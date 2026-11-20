package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
)

const (
	matchThresholdSettingKey    = "recognition_match_threshold"
	detectionThresholdSettingKey = "recognition_detection_threshold"
	livenessPassSettingKey      = "recognition_liveness_pass_threshold"
	livenessFrameSettingKey     = "recognition_liveness_frame_threshold"
	livenessSpoofSettingKey     = "recognition_liveness_spoof_threshold"
)

// RecognitionThresholds stores only parameters that are actually used by the
// recognition and passive-liveness pipelines.
type RecognitionThresholds struct {
	MatchThreshold          float64 `json:"match_threshold"`
	DetectionThreshold      float64 `json:"detection_threshold"`
	LivenessPassThreshold   float64 `json:"liveness_pass_threshold"`
	LivenessFrameThreshold  float64 `json:"liveness_frame_threshold"`
	LivenessSpoofThreshold  float64 `json:"liveness_spoof_threshold"`
}

func (s *Store) LoadRecognitionThresholds(ctx context.Context, defaults RecognitionThresholds) (RecognitionThresholds, error) {
	values := []struct {
		key string
		dst *float64
	}{
		{matchThresholdSettingKey, &defaults.MatchThreshold},
		{detectionThresholdSettingKey, &defaults.DetectionThreshold},
		{livenessPassSettingKey, &defaults.LivenessPassThreshold},
		{livenessFrameSettingKey, &defaults.LivenessFrameThreshold},
		{livenessSpoofSettingKey, &defaults.LivenessSpoofThreshold},
	}
	for _, item := range values {
		var raw string
		err := s.db.QueryRowContext(ctx, "SELECT value FROM app_settings WHERE key=?", item.key).Scan(&raw)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return RecognitionThresholds{}, err
		}
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return RecognitionThresholds{}, err
		}
		*item.dst = value
	}
	return defaults, nil
}

func (s *Store) SaveRecognitionThresholds(ctx context.Context, value RecognitionThresholds) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, item := range []struct {
		key string
		value float64
	}{
		{matchThresholdSettingKey, value.MatchThreshold},
		{detectionThresholdSettingKey, value.DetectionThreshold},
		{livenessPassSettingKey, value.LivenessPassThreshold},
		{livenessFrameSettingKey, value.LivenessFrameThreshold},
		{livenessSpoofSettingKey, value.LivenessSpoofThreshold},
	} {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO app_settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value",
			item.key, strconv.FormatFloat(item.value, 'f', -1, 64),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ResetRecognitionThresholds removes only the five debug overrides. Other
// settings, accounts, samples, and attendance are left untouched.
func (s *Store) ResetRecognitionThresholds(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM app_settings WHERE key IN (?,?,?,?,?)`,
		matchThresholdSettingKey,
		detectionThresholdSettingKey,
		livenessPassSettingKey,
		livenessFrameSettingKey,
		livenessSpoofSettingKey,
	)
	return err
}
