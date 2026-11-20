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

// SaveAttendanceSnapshot appends one JPEG to the attendance day's photo timeline.
func (s *Store) SaveAttendanceSnapshot(ctx context.Context, attendanceID int64, jpegData []byte) (AttendanceSnapshot, error) {
	if attendanceID <= 0 {
		return AttendanceSnapshot{}, errors.New("invalid attendance id")
	}
	if len(jpegData) == 0 || len(jpegData) > maxAttendanceSnapshotBytes {
		return AttendanceSnapshot{}, fmt.Errorf("attendance snapshot must be between 1 byte and %d bytes", maxAttendanceSnapshotBytes)
	}
	if s.snapshotDir == "" {
		return AttendanceSnapshot{}, errors.New("attendance snapshot directory is unavailable")
	}
	now := time.Now()
	fileName := fmt.Sprintf("attendance-%d-%d.jpg", attendanceID, now.UnixNano())
	temp, err := os.CreateTemp(s.snapshotDir, ".facesign-snapshot-*.tmp")
	if err != nil {
		return AttendanceSnapshot{}, err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.Write(jpegData); err != nil {
		_ = temp.Close()
		return AttendanceSnapshot{}, err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return AttendanceSnapshot{}, err
	}
	if err := temp.Close(); err != nil {
		return AttendanceSnapshot{}, err
	}
	finalPath := filepath.Join(s.snapshotDir, fileName)
	if err := os.Rename(tempName, finalPath); err != nil {
		return AttendanceSnapshot{}, err
	}

	result, err := s.db.ExecContext(ctx,
		"INSERT INTO attendance_snapshots(attendance_id,file_name,recognized_at) VALUES(?,?,?)",
		attendanceID, fileName, now.UnixNano(),
	)
	if err != nil {
		_ = os.Remove(finalPath)
		return AttendanceSnapshot{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return AttendanceSnapshot{}, err
	}
	return AttendanceSnapshot{
		ID: id, AttendanceID: attendanceID,
		RecognizedAt: now.Format("2006-01-02 15:04:05.000"),
	}, nil
}

func validAttendanceSnapshotName(name string) bool {
	return name != "" && filepath.Base(name) == name &&
		strings.HasPrefix(name, "attendance-") && strings.HasSuffix(name, ".jpg")
}

func (s *Store) ListAttendanceSnapshots(ctx context.Context, attendanceID int64) ([]AttendanceSnapshot, error) {
	if attendanceID <= 0 {
		return nil, errors.New("invalid attendance id")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id,attendance_id,recognized_at
		FROM attendance_snapshots
		WHERE attendance_id=?
		ORDER BY recognized_at DESC,id DESC`, attendanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AttendanceSnapshot, 0)
	for rows.Next() {
		var item AttendanceSnapshot
		var recognizedAt int64
		if err := rows.Scan(&item.ID, &item.AttendanceID, &recognizedAt); err != nil {
			return nil, err
		}
		item.RecognizedAt = time.Unix(0, recognizedAt).Local().Format("2006-01-02 15:04:05.000")
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) AttendanceSnapshotPath(ctx context.Context, snapshotID int64) (string, error) {
	if snapshotID <= 0 {
		return "", os.ErrNotExist
	}
	var fileName string
	err := s.db.QueryRowContext(ctx,
		"SELECT file_name FROM attendance_snapshots WHERE id=?", snapshotID,
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
