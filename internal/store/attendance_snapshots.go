package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxAttendanceSnapshotBytes = 8 << 20

type AttendanceSnapshotCacheStats struct {
	Count int   `json:"count"`
	Bytes int64 `json:"bytes"`
}

// SaveAttendanceSnapshot stores one JPEG for an attendance record. Repeated
// recognition for the same student/day keeps the first available photograph.
func (s *Store) SaveAttendanceSnapshot(ctx context.Context, attendanceID int64, jpegData []byte) (bool, error) {
	if attendanceID <= 0 {
		return false, errors.New("invalid attendance id")
	}
	if len(jpegData) == 0 || len(jpegData) > maxAttendanceSnapshotBytes {
		return false, fmt.Errorf("attendance snapshot must be between 1 byte and %d bytes", maxAttendanceSnapshotBytes)
	}
	if s.snapshotDir == "" {
		return false, errors.New("attendance snapshot directory is unavailable")
	}

	var existing string
	err := s.db.QueryRowContext(ctx,
		"SELECT file_name FROM attendance_snapshots WHERE attendance_id=?", attendanceID,
	).Scan(&existing)
	if err == nil && validAttendanceSnapshotName(existing) {
		if info, statErr := os.Stat(filepath.Join(s.snapshotDir, existing)); statErr == nil && info.Mode().IsRegular() {
			return true, nil
		}
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}

	temp, err := os.CreateTemp(s.snapshotDir, ".facesign-snapshot-*.tmp")
	if err != nil {
		return false, err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.Write(jpegData); err != nil {
		_ = temp.Close()
		return false, err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return false, err
	}
	if err := temp.Close(); err != nil {
		return false, err
	}

	fileName := fmt.Sprintf("attendance-%d.jpg", attendanceID)
	finalPath := filepath.Join(s.snapshotDir, fileName)
	if err := os.Rename(tempName, finalPath); err != nil {
		// Another request may have won the same attendance-id race.
		if info, statErr := os.Stat(finalPath); statErr != nil || !info.Mode().IsRegular() {
			return false, err
		}
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO attendance_snapshots(attendance_id,file_name,created_at)
		VALUES(?,?,?)
		ON CONFLICT(attendance_id) DO UPDATE SET
			file_name=excluded.file_name,
			created_at=excluded.created_at`,
		attendanceID, fileName, time.Now().Unix(),
	)
	if err != nil {
		return false, err
	}
	return true, nil
}

func validAttendanceSnapshotName(name string) bool {
	return name != "" && filepath.Base(name) == name &&
		strings.HasPrefix(name, "attendance-") && strings.HasSuffix(name, ".jpg")
}

func (s *Store) AttendanceSnapshotPath(ctx context.Context, attendanceID int64) (string, error) {
	if attendanceID <= 0 {
		return "", os.ErrNotExist
	}
	var fileName string
	err := s.db.QueryRowContext(ctx,
		"SELECT file_name FROM attendance_snapshots WHERE attendance_id=?", attendanceID,
	).Scan(&fileName)
	if errors.Is(err, sql.ErrNoRows) {
		return "", os.ErrNotExist
	}
	if err != nil {
		return "", err
	}
	if !validAttendanceSnapshotName(fileName) {
		return "", os.ErrNotExist
	}
	path := filepath.Join(s.snapshotDir, fileName)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", os.ErrNotExist
		}
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", os.ErrNotExist
	}
	return path, nil
}

func (s *Store) AttendanceSnapshotCacheStats(ctx context.Context) (AttendanceSnapshotCacheStats, error) {
	if err := ctx.Err(); err != nil {
		return AttendanceSnapshotCacheStats{}, err
	}
	entries, err := os.ReadDir(s.snapshotDir)
	if err != nil {
		return AttendanceSnapshotCacheStats{}, err
	}
	var stats AttendanceSnapshotCacheStats
	for _, entry := range entries {
		if entry.IsDir() || !validAttendanceSnapshotName(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return AttendanceSnapshotCacheStats{}, err
		}
		stats.Count++
		stats.Bytes += info.Size()
	}
	return stats, nil
}

// ClearAttendanceSnapshots removes only local attendance photographs and
// their index rows. Attendance, student, face-sample and account data remain.
func (s *Store) ClearAttendanceSnapshots(ctx context.Context) (AttendanceSnapshotCacheStats, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AttendanceSnapshotCacheStats{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM attendance_snapshots"); err != nil {
		return AttendanceSnapshotCacheStats{}, err
	}
	if err := tx.Commit(); err != nil {
		return AttendanceSnapshotCacheStats{}, err
	}

	entries, err := os.ReadDir(s.snapshotDir)
	if err != nil {
		return AttendanceSnapshotCacheStats{}, err
	}
	var removed AttendanceSnapshotCacheStats
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		isSnapshot := validAttendanceSnapshotName(name)
		isTemp := strings.HasPrefix(name, ".facesign-snapshot-") && strings.HasSuffix(name, ".tmp")
		if !isSnapshot && !isTemp {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return removed, infoErr
		}
		if err := os.Remove(filepath.Join(s.snapshotDir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return removed, err
		}
		if isSnapshot {
			removed.Count++
			removed.Bytes += info.Size()
		}
	}
	return removed, nil
}
