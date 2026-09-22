package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type CameraInput struct {
	Name        string
	Kind        string
	DeviceID    string
	Protocol    string
	StreamURL   string
	SnapshotURL string
	Username    string
	Password    string
	AuthMode    string
	Width       int
	Height      int
	FPS         int
	TimeoutMS   int
	TLSInsecure bool
	IsDefault   bool
}

type rowScanner interface {
	Scan(dest ...any) error
}

func normalizeCameraInput(in CameraInput) (CameraInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return CameraInput{}, errors.New("摄像头名称不能为空")
	}
	if len([]rune(in.Name)) > 80 {
		return CameraInput{}, errors.New("摄像头名称不能超过80个字符")
	}
	in.Kind = strings.ToLower(strings.TrimSpace(in.Kind))
	in.DeviceID = strings.TrimSpace(in.DeviceID)
	in.Protocol = strings.ToLower(strings.TrimSpace(in.Protocol))
	in.StreamURL = strings.TrimSpace(in.StreamURL)
	in.SnapshotURL = strings.TrimSpace(in.SnapshotURL)
	in.Username = strings.TrimSpace(in.Username)
	in.AuthMode = strings.ToLower(strings.TrimSpace(in.AuthMode))

	if in.Width == 0 {
		in.Width = 1280
	}
	if in.Height == 0 {
		in.Height = 720
	}
	if in.Width < 320 || in.Width > 7680 || in.Height < 240 || in.Height > 4320 {
		return CameraInput{}, errors.New("摄像头分辨率超出允许范围")
	}
	if in.TimeoutMS == 0 {
		in.TimeoutMS = 3000
	}
	if in.TimeoutMS < 500 || in.TimeoutMS > 30000 {
		return CameraInput{}, errors.New("网络超时必须在500到30000毫秒之间")
	}

	switch in.Kind {
	case "local":
		in.Protocol = "browser"
		in.StreamURL = ""
		in.SnapshotURL = ""
		in.Username = ""
		in.Password = ""
		in.AuthMode = "none"
		in.TLSInsecure = false
		if in.FPS == 0 {
			in.FPS = 30
		}
		if in.FPS < 1 || in.FPS > 60 {
			return CameraInput{}, errors.New("本机摄像头帧率必须在1到60之间")
		}
	case "network":
		if in.FPS == 0 {
			in.FPS = 5
		}
		if in.FPS < 1 || in.FPS > 30 {
			return CameraInput{}, errors.New("网络摄像头帧率必须在1到30之间")
		}
		switch in.Protocol {
		case "http_snapshot":
			if err := validateCameraURL(in.SnapshotURL, "http", "https"); err != nil {
				return CameraInput{}, fmt.Errorf("抓图地址: %w", err)
			}
		case "mjpeg":
			if err := validateCameraURL(in.StreamURL, "http", "https"); err != nil {
				return CameraInput{}, fmt.Errorf("MJPEG地址: %w", err)
			}
			if in.SnapshotURL != "" {
				if err := validateCameraURL(in.SnapshotURL, "http", "https"); err != nil {
					return CameraInput{}, fmt.Errorf("抓图地址: %w", err)
				}
			}
		case "rtsp":
			if err := validateCameraURL(in.StreamURL, "rtsp", "rtsps"); err != nil {
				return CameraInput{}, fmt.Errorf("RTSP地址: %w", err)
			}
			if err := validateCameraURL(in.SnapshotURL, "http", "https"); err != nil {
				return CameraInput{}, fmt.Errorf("RTSP摄像头还需要HTTP/HTTPS抓图地址: %w", err)
			}
		default:
			return CameraInput{}, errors.New("不支持的网络摄像头协议")
		}
		switch in.AuthMode {
		case "", "none":
			in.AuthMode = "none"
			in.Username = ""
			in.Password = ""
		case "basic", "digest":
			if in.Username == "" {
				return CameraInput{}, errors.New("启用摄像头认证时必须填写用户名")
			}
		default:
			return CameraInput{}, errors.New("认证方式只支持无认证、Basic或Digest")
		}
	default:
		return CameraInput{}, errors.New("摄像头类型只支持本机或网络摄像头")
	}
	return in, nil
}

func validateCameraURL(raw string, allowed ...string) error {
	if strings.TrimSpace(raw) == "" {
		return errors.New("地址不能为空")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return errors.New("地址格式不正确")
	}
	if u.User != nil {
		return errors.New("地址中不要包含账号密码，请使用独立的用户名和密码字段")
	}
	scheme := strings.ToLower(u.Scheme)
	for _, item := range allowed {
		if scheme == item {
			return nil
		}
	}
	return fmt.Errorf("协议必须是 %s", strings.Join(allowed, "/"))
}

func scanCamera(scanner rowScanner) (Camera, error) {
	var item Camera
	var tlsInsecure, isDefault int
	err := scanner.Scan(
		&item.ID, &item.Name, &item.Kind, &item.DeviceID, &item.Protocol,
		&item.StreamURL, &item.SnapshotURL, &item.Username, &item.Password, &item.AuthMode,
		&item.Width, &item.Height, &item.FPS, &item.TimeoutMS, &tlsInsecure, &isDefault,
	)
	if err != nil {
		return Camera{}, err
	}
	item.TLSInsecure = tlsInsecure != 0
	item.IsDefault = isDefault != 0
	item.HasPassword = item.Password != ""
	return item, nil
}

const cameraSelectColumns = "id,name,kind,device_id,protocol,stream_url,snapshot_url,username,password,auth_mode,width,height,fps,timeout_ms,tls_insecure,is_default"

func (s *Store) ListCameras(ctx context.Context) ([]Camera, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+cameraSelectColumns+" FROM cameras ORDER BY is_default DESC,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Camera, 0)
	for rows.Next() {
		item, err := scanCamera(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) CameraByID(ctx context.Context, id int64) (Camera, error) {
	return scanCamera(s.db.QueryRowContext(ctx, "SELECT "+cameraSelectColumns+" FROM cameras WHERE id=?", id))
}

func (s *Store) DefaultCamera(ctx context.Context) (Camera, error) {
	return scanCamera(s.db.QueryRowContext(ctx, "SELECT "+cameraSelectColumns+" FROM cameras WHERE is_default=1 ORDER BY id LIMIT 1"))
}

func (s *Store) CreateCamera(ctx context.Context, input CameraInput) (Camera, error) {
	input, err := normalizeCameraInput(input)
	if err != nil {
		return Camera{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Camera{}, err
	}
	defer tx.Rollback()

	var count int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM cameras").Scan(&count); err != nil {
		return Camera{}, err
	}
	makeDefault := input.IsDefault || count == 0
	if makeDefault {
		if _, err := tx.ExecContext(ctx, "UPDATE cameras SET is_default=0,updated_at=?", time.Now().Unix()); err != nil {
			return Camera{}, err
		}
	}
	now := time.Now().Unix()
	result, err := tx.ExecContext(ctx, `
		INSERT INTO cameras(
			name,kind,device_id,protocol,stream_url,snapshot_url,username,password,auth_mode,
			width,height,fps,timeout_ms,tls_insecure,is_default,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		input.Name, input.Kind, input.DeviceID, input.Protocol, input.StreamURL, input.SnapshotURL,
		input.Username, input.Password, input.AuthMode, input.Width, input.Height, input.FPS,
		input.TimeoutMS, boolInt(input.TLSInsecure), boolInt(makeDefault), now, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "cameras.name") {
			return Camera{}, errors.New("该摄像头名称已经存在")
		}
		return Camera{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Camera{}, err
	}
	if err := tx.Commit(); err != nil {
		return Camera{}, err
	}
	return s.CameraByID(ctx, id)
}

func (s *Store) UpdateCamera(ctx context.Context, id int64, input CameraInput) (Camera, error) {
	input, err := normalizeCameraInput(input)
	if err != nil {
		return Camera{}, err
	}
	current, err := s.CameraByID(ctx, id)
	if err != nil {
		return Camera{}, err
	}
	makeDefault := current.IsDefault || input.IsDefault

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Camera{}, err
	}
	defer tx.Rollback()
	now := time.Now().Unix()
	if makeDefault {
		if _, err := tx.ExecContext(ctx, "UPDATE cameras SET is_default=0,updated_at=? WHERE id<>?", now, id); err != nil {
			return Camera{}, err
		}
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE cameras SET
			name=?,kind=?,device_id=?,protocol=?,stream_url=?,snapshot_url=?,username=?,password=?,auth_mode=?,
			width=?,height=?,fps=?,timeout_ms=?,tls_insecure=?,is_default=?,updated_at=?
		WHERE id=?`,
		input.Name, input.Kind, input.DeviceID, input.Protocol, input.StreamURL, input.SnapshotURL,
		input.Username, input.Password, input.AuthMode, input.Width, input.Height, input.FPS,
		input.TimeoutMS, boolInt(input.TLSInsecure), boolInt(makeDefault), now, id,
	)
	if err != nil {
		if strings.Contains(err.Error(), "cameras.name") {
			return Camera{}, errors.New("该摄像头名称已经存在")
		}
		return Camera{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return Camera{}, err
	}
	if affected == 0 {
		return Camera{}, sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return Camera{}, err
	}
	return s.CameraByID(ctx, id)
}

func (s *Store) SetDefaultCamera(ctx context.Context, id int64) (Camera, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Camera{}, err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM cameras WHERE id=?", id).Scan(&exists); err != nil {
		return Camera{}, err
	}
	if exists == 0 {
		return Camera{}, sql.ErrNoRows
	}
	now := time.Now().Unix()
	if _, err := tx.ExecContext(ctx, "UPDATE cameras SET is_default=0,updated_at=? WHERE is_default<>0", now); err != nil {
		return Camera{}, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE cameras SET is_default=1,updated_at=? WHERE id=?", now, id); err != nil {
		return Camera{}, err
	}
	if err := tx.Commit(); err != nil {
		return Camera{}, err
	}
	return s.CameraByID(ctx, id)
}

func (s *Store) DeleteCamera(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var wasDefault int
	if err := tx.QueryRowContext(ctx, "SELECT is_default FROM cameras WHERE id=?", id).Scan(&wasDefault); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, "DELETE FROM cameras WHERE id=?", id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	if wasDefault != 0 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE cameras SET is_default=1,updated_at=?
			WHERE id=(SELECT id FROM cameras ORDER BY id LIMIT 1)`, time.Now().Unix()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
