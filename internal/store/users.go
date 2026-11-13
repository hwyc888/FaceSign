package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *Store) UserCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}

func (s *Store) CreateInitialAdmin(ctx context.Context, username, passwordHash string) (User, error) {
	if username == "" || passwordHash == "" {
		return User{}, errors.New("管理员用户名和密码不能为空")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()

	var count int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return User{}, err
	}
	if count != 0 {
		return User{}, errors.New("管理员已经初始化，不能再次创建")
	}
	now := time.Now().Unix()
	result, err := tx.ExecContext(ctx, "INSERT INTO users(username,password_hash,is_admin,can_edit_students,can_add_students,can_resample_faces,can_arrange_seats,created_at,updated_at) VALUES(?,?,1,1,1,1,1,?,?)", username, passwordHash, now, now)
	if err != nil {
		return User{}, fmt.Errorf("创建管理员失败: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return User{}, err
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	return User{ID:id, Username:username, IsAdmin:true, CanEditStudents:true, CanAddStudents:true, CanResampleFaces:true, CanArrangeSeats:true}, nil
}

func (s *Store) UserByUsername(ctx context.Context, username string) (User, error) {
	var user User
	err := s.db.QueryRowContext(ctx, "SELECT id,username,password_hash,is_admin,can_edit_students,can_add_students,can_resample_faces,can_arrange_seats FROM users WHERE username=?", username).Scan(
		&user.ID, &user.Username, &user.PasswordHash, &user.IsAdmin,
		&user.CanEditStudents, &user.CanAddStudents, &user.CanResampleFaces, &user.CanArrangeSeats,
	)
	return user, err
}

func (s *Store) UserBySession(ctx context.Context, tokenHash string, now time.Time) (User, error) {
	var user User
	err := s.db.QueryRowContext(ctx, "SELECT u.id,u.username,u.password_hash,u.is_admin,u.can_edit_students,u.can_add_students,u.can_resample_faces,u.can_arrange_seats FROM auth_sessions a JOIN users u ON u.id=a.user_id WHERE a.token_hash=? AND a.expires_at>?", tokenHash, now.Unix()).Scan(
		&user.ID, &user.Username, &user.PasswordHash, &user.IsAdmin,
		&user.CanEditStudents, &user.CanAddStudents, &user.CanResampleFaces, &user.CanArrangeSeats,
	)
	return user, err
}

func (s *Store) UserBySessionForRelease(ctx context.Context, tokenHash string) (User, error) {
	var user User
	err := s.db.QueryRowContext(ctx, "SELECT u.id,u.username,u.password_hash,u.is_admin,u.can_edit_students,u.can_add_students,u.can_resample_faces,u.can_arrange_seats FROM auth_sessions a JOIN users u ON u.id=a.user_id WHERE a.token_hash=?", tokenHash).Scan(
		&user.ID, &user.Username, &user.PasswordHash, &user.IsAdmin,
		&user.CanEditStudents, &user.CanAddStudents, &user.CanResampleFaces, &user.CanArrangeSeats,
	)
	return user, err
}

func (s *Store) CreateAuthSession(ctx context.Context, tokenHash string, userID int64, expiresAt time.Time) error {
	now := time.Now().Unix()
	expiredCleanupBefore := now - int64((7 * 24 * time.Hour).Seconds())
	if _, err := s.db.ExecContext(ctx, "DELETE FROM auth_sessions WHERE expires_at<=?", expiredCleanupBefore); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, "INSERT INTO auth_sessions(token_hash,user_id,expires_at,created_at) VALUES(?,?,?,?)", tokenHash, userID, expiresAt.Unix(), now)
	return err
}

func (s *Store) DeleteAuthSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM auth_sessions WHERE token_hash=?", tokenHash)
	return err
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,username,is_admin,can_edit_students,can_add_students,can_resample_faces,can_arrange_seats FROM users ORDER BY is_admin DESC,username COLLATE NOCASE")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make([]User, 0)
	for rows.Next() {
		var user User
		if err := rows.Scan(
			&user.ID, &user.Username, &user.IsAdmin, &user.CanEditStudents,
			&user.CanAddStudents, &user.CanResampleFaces, &user.CanArrangeSeats,
		); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) CreateUser(ctx context.Context, username, passwordHash string, user User) (User, error) {
	now := time.Now().Unix()
	result, err := s.db.ExecContext(ctx, "INSERT INTO users(username,password_hash,is_admin,can_edit_students,can_add_students,can_resample_faces,can_arrange_seats,created_at,updated_at) VALUES(?,?,0,?,?,?,?,?,?)",
		username, passwordHash, user.CanEditStudents, user.CanAddStudents,
		user.CanResampleFaces, user.CanArrangeSeats, now, now)
	if err != nil {
		return User{}, fmt.Errorf("创建用户失败: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return User{}, err
	}
	user.ID = id
	user.Username = username
	user.IsAdmin = false
	return user, nil
}

func (s *Store) CloneUsers(ctx context.Context, sourceID int64, credentials []UserCloneCredential) ([]User, error) {
	if sourceID <= 0 {
		return nil, errors.New("无效的源用户")
	}
	if len(credentials) == 0 || len(credentials) > 100 {
		return nil, errors.New("一次只能复制 1 到 100 个账号")
	}

	seen := make(map[string]struct{}, len(credentials))
	for _, credential := range credentials {
		username := strings.TrimSpace(credential.Username)
		if username == "" || username != credential.Username || credential.PasswordHash == "" {
			return nil, errors.New("复制账号信息不完整")
		}
		key := strings.ToLower(username)
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("用户名重复: %s", username)
		}
		seen[key] = struct{}{}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var source User
	err = tx.QueryRowContext(ctx, `
		SELECT id,username,is_admin,can_edit_students,can_add_students,can_resample_faces,can_arrange_seats
		FROM users WHERE id=?`, sourceID).Scan(
		&source.ID, &source.Username, &source.IsAdmin,
		&source.CanEditStudents, &source.CanAddStudents,
		&source.CanResampleFaces, &source.CanArrangeSeats,
	)
	if err != nil {
		return nil, err
	}
	if source.IsAdmin {
		return nil, errors.New("不能复制管理员账号")
	}

	now := time.Now().Unix()
	created := make([]User, 0, len(credentials))
	for _, credential := range credentials {
		var exists int
		if err := tx.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM users WHERE username=? COLLATE NOCASE",
			credential.Username,
		).Scan(&exists); err != nil {
			return nil, err
		}
		if exists > 0 {
			return nil, fmt.Errorf("用户名已存在: %s", credential.Username)
		}

		result, err := tx.ExecContext(ctx, `
			INSERT INTO users(username,password_hash,is_admin,can_edit_students,can_add_students,can_resample_faces,can_arrange_seats,created_at,updated_at)
			VALUES(?,?,0,?,?,?,?,?,?)`,
			credential.Username, credential.PasswordHash,
			source.CanEditStudents, source.CanAddStudents,
			source.CanResampleFaces, source.CanArrangeSeats, now, now,
		)
		if err != nil {
			return nil, fmt.Errorf("复制用户 %s 失败: %w", credential.Username, err)
		}
		id, err := result.LastInsertId()
		if err != nil {
			return nil, err
		}
		created = append(created, User{
			ID: id, Username: credential.Username, IsAdmin: false,
			CanEditStudents: source.CanEditStudents,
			CanAddStudents: source.CanAddStudents,
			CanResampleFaces: source.CanResampleFaces,
			CanArrangeSeats: source.CanArrangeSeats,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}

func (s *Store) UpdateUserAccess(ctx context.Context, id int64, user User, passwordHash string) error {
	result, err := s.db.ExecContext(ctx, "UPDATE users SET can_edit_students=?,can_add_students=?,can_resample_faces=?,can_arrange_seats=?,password_hash=CASE WHEN ?='' THEN password_hash ELSE ? END,updated_at=? WHERE id=? AND is_admin=0",
		user.CanEditStudents, user.CanAddStudents, user.CanResampleFaces, user.CanArrangeSeats,
		passwordHash, passwordHash, time.Now().Unix(), id)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		var isAdmin bool
		if err := s.db.QueryRowContext(ctx, "SELECT is_admin FROM users WHERE id=?", id).Scan(&isAdmin); err != nil {
			return err
		}
		if isAdmin {
			return errors.New("管理员账号权限不可修改")
		}
		return sql.ErrNoRows
	}
	if passwordHash != "" {
		if _, err := s.db.ExecContext(ctx, "UPDATE auth_sessions SET expires_at=MIN(expires_at,?) WHERE user_id=?", time.Now().Unix(), id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	var isAdmin bool
	if err := s.db.QueryRowContext(ctx, "SELECT is_admin FROM users WHERE id=?", id).Scan(&isAdmin); err != nil {
		return err
	}
	if isAdmin {
		return errors.New("管理员账号不能删除")
	}
	result, err := s.db.ExecContext(ctx, "DELETE FROM users WHERE id=? AND is_admin=0", id)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ExpireOtherAuthSessions(ctx context.Context, userID int64, keepTokenHash string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE auth_sessions SET expires_at=MIN(expires_at,?) WHERE user_id=? AND token_hash<>?", time.Now().Unix(), userID, keepTokenHash)
	return err
}

func (s *Store) UpdateUserPassword(ctx context.Context, id int64, passwordHash string) error {
	result, err := s.db.ExecContext(ctx, "UPDATE users SET password_hash=?,updated_at=? WHERE id=?", passwordHash, time.Now().Unix(), id)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return sql.ErrNoRows
	}
	return nil
}
