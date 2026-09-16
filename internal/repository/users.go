package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hwyc888/FaceSign/internal/domain"
)

func (r *Repository) UserCount(ctx context.Context) (int, error) {
	var count int
	return count, r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
}

func (r *Repository) CreateUser(ctx context.Context, username, passwordHash, displayName, role string) (domain.User, error) {
	username = strings.TrimSpace(username)
	displayName = strings.TrimSpace(displayName)
	if username == "" || displayName == "" {
		return domain.User{}, fmt.Errorf("username and display name are required")
	}
	if role != domain.RoleAdmin && role != domain.RoleTeacher {
		return domain.User{}, fmt.Errorf("invalid role")
	}
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO users(username, password_hash, display_name, role, created_at) VALUES(?,?,?,?,?)`,
		username, passwordHash, displayName, role, time.Now().Unix(),
	)
	if err != nil {
		return domain.User{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return domain.User{}, err
	}
	return domain.User{ID: id, Username: username, DisplayName: displayName, Role: role}, nil
}

func (r *Repository) FindUserByUsername(ctx context.Context, username string) (domain.User, string, error) {
	var user domain.User
	var passwordHash string
	err := r.db.QueryRowContext(ctx,
		`SELECT id, username, display_name, role, password_hash FROM users WHERE username=?`,
		strings.TrimSpace(username),
	).Scan(&user.ID, &user.Username, &user.DisplayName, &user.Role, &passwordHash)
	if err != nil {
		return domain.User{}, "", err
	}
	return user, passwordHash, nil
}

func (r *Repository) ListUsers(ctx context.Context) ([]domain.User, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, username, display_name, role FROM users ORDER BY display_name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []domain.User
	for rows.Next() {
		var user domain.User
		if err := rows.Scan(&user.ID, &user.Username, &user.DisplayName, &user.Role); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (r *Repository) CreateLoginSession(ctx context.Context, tokenHash string, userID int64, expiresAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO login_sessions(token_hash, user_id, expires_at, created_at) VALUES(?,?,?,?)`,
		tokenHash, userID, expiresAt.Unix(), time.Now().Unix(),
	)
	return err
}

func (r *Repository) FindUserBySession(ctx context.Context, tokenHash string, now time.Time) (domain.User, error) {
	var user domain.User
	err := r.db.QueryRowContext(ctx, `
		SELECT u.id, u.username, u.display_name, u.role
		FROM login_sessions s
		JOIN users u ON u.id=s.user_id
		WHERE s.token_hash=? AND s.expires_at>?`,
		tokenHash, now.Unix(),
	).Scan(&user.ID, &user.Username, &user.DisplayName, &user.Role)
	return user, err
}

func (r *Repository) DeleteLoginSession(ctx context.Context, tokenHash string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM login_sessions WHERE token_hash=?`, tokenHash)
	return err
}

func (r *Repository) DeleteExpiredLoginSessions(ctx context.Context, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM login_sessions WHERE expires_at<=?`, now.Unix())
	return err
}

func IsNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
