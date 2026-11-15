package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
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
	lowLightRecognitionModeSettingKey   = "low_light_recognition_mode"
	disableLivenessDetectionSettingKey  = "disable_liveness_detection"
	faceDetectionThresholdSettingKey   = "face_detection_threshold"
	livenessPassThresholdSettingKey     = "liveness_pass_threshold"
	livenessFrameThresholdSettingKey    = "liveness_frame_threshold"
	livenessSpoofThresholdSettingKey    = "liveness_spoof_threshold"
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
	LowLightRecognitionMode    bool `json:"low_light_recognition_mode"`
	DisableLivenessDetection   bool    `json:"disable_liveness_detection"`
	FaceDetectionThreshold     float64 `json:"face_detection_threshold"`
	LivenessPassThreshold      float64 `json:"liveness_pass_threshold"`
	LivenessFrameThreshold     float64 `json:"liveness_frame_threshold"`
	LivenessSpoofThreshold     float64 `json:"liveness_spoof_threshold"`
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
	lowLightMode, err := s.appSettingBool(ctx, lowLightRecognitionModeSettingKey, false)
	if err != nil {
		return AppSettings{}, err
	}
	disableLiveness, err := s.appSettingBool(ctx, disableLivenessDetectionSettingKey, false)
	if err != nil {
		return AppSettings{}, err
	}
	faceDetection, err := s.appSettingFloat(ctx, faceDetectionThresholdSettingKey, 0.45)
	if err != nil {
		return AppSettings{}, err
	}
	livenessPass, err := s.appSettingFloat(ctx, livenessPassThresholdSettingKey, 0.60)
	if err != nil {
		return AppSettings{}, err
	}
	livenessFrame, err := s.appSettingFloat(ctx, livenessFrameThresholdSettingKey, 0.55)
	if err != nil {
		return AppSettings{}, err
	}
	livenessSpoof, err := s.appSettingFloat(ctx, livenessSpoofThresholdSettingKey, 0.40)
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
		LowLightRecognitionMode:   lowLightMode,
		DisableLivenessDetection:  disableLiveness,
		FaceDetectionThreshold:    faceDetection,
		LivenessPassThreshold:     livenessPass,
		LivenessFrameThreshold:    livenessFrame,
		LivenessSpoofThreshold:    livenessSpoof,
	}, nil
}

func (s *Store) LowLightRecognitionMode(ctx context.Context) (bool, error) {
	return s.appSettingBool(ctx, lowLightRecognitionModeSettingKey, false)
}

func (s *Store) DisableLivenessDetection(ctx context.Context) (bool, error) {
	return s.appSettingBool(ctx, disableLivenessDetectionSettingKey, false)
}

func (s *Store) appSettingFloat(ctx context.Context, key string, defaultValue float64) (float64, error) {
	var value float64
	err := s.db.QueryRowContext(ctx,
		"SELECT CAST(value AS REAL) FROM app_settings WHERE key=?",
		key,
	).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultValue, nil
	}
	if err != nil {
		return 0, err
	}
	return value, nil
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
		{lowLightRecognitionModeSettingKey, settings.LowLightRecognitionMode},
		{disableLivenessDetectionSettingKey, settings.DisableLivenessDetection},
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
	floatValues := []struct {
		key   string
		value float64
	}{
		{faceDetectionThresholdSettingKey, settings.FaceDetectionThreshold},
		{livenessPassThresholdSettingKey, settings.LivenessPassThreshold},
		{livenessFrameThresholdSettingKey, settings.LivenessFrameThreshold},
		{livenessSpoofThresholdSettingKey, settings.LivenessSpoofThreshold},
	}
	for _, item := range floatValues {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO app_settings(key,value) VALUES(?,?)
			ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
			item.key, strconv.FormatFloat(item.value, 'f', 4, 64),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}
