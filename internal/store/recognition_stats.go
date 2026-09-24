package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	recognitionStatVerified     = "verified"
	recognitionStatUnregistered = "unregistered"
)

type RecognitionStats struct {
	Verified     int `json:"verified"`
	Unregistered int `json:"unregistered"`
}

func (s *Store) RecognitionStats(ctx context.Context) (RecognitionStats, error) {
	var stats RecognitionStats
	rows, err := s.db.QueryContext(ctx, `
		SELECT kind, COUNT(*)
		FROM recognition_stats_seen
		GROUP BY kind`)
	if err != nil {
		return stats, err
	}
	defer rows.Close()

	for rows.Next() {
		var kind string
		var count int
		if err := rows.Scan(&kind, &count); err != nil {
			return stats, err
		}
		switch kind {
		case recognitionStatVerified:
			stats.Verified = count
		case recognitionStatUnregistered:
			stats.Unregistered = count
		}
	}
	return stats, rows.Err()
}

func (s *Store) RecordRecognitionStats(ctx context.Context, studentIDs []int64, unknownTrackIDs []string) (RecognitionStats, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RecognitionStats{}, err
	}
	defer tx.Rollback()

	now := time.Now().Unix()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO recognition_stats_seen(kind, subject_key, created_at)
		VALUES(?,?,?)`)
	if err != nil {
		return RecognitionStats{}, err
	}
	defer stmt.Close()

	for _, studentID := range studentIDs {
		if studentID <= 0 {
			continue
		}
		if _, err := stmt.ExecContext(ctx, recognitionStatVerified, "student:"+strconv.FormatInt(studentID, 10), now); err != nil {
			return RecognitionStats{}, err
		}
	}
	for _, trackID := range unknownTrackIDs {
		trackID = strings.TrimSpace(trackID)
		if trackID == "" {
			continue
		}
		if _, err := stmt.ExecContext(ctx, recognitionStatUnregistered, "track:"+trackID, now); err != nil {
			return RecognitionStats{}, err
		}
	}

	stats, err := recognitionStatsTx(ctx, tx)
	if err != nil {
		return RecognitionStats{}, err
	}
	if err := tx.Commit(); err != nil {
		return RecognitionStats{}, err
	}
	return stats, nil
}

func (s *Store) ClearRecognitionStats(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM recognition_stats_seen")
	return err
}

func recognitionStatsTx(ctx context.Context, tx *sql.Tx) (RecognitionStats, error) {
	var stats RecognitionStats
	rows, err := tx.QueryContext(ctx, `
		SELECT kind, COUNT(*)
		FROM recognition_stats_seen
		GROUP BY kind`)
	if err != nil {
		return stats, fmt.Errorf("query recognition stats: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var kind string
		var count int
		if err := rows.Scan(&kind, &count); err != nil {
			return stats, err
		}
		switch kind {
		case recognitionStatVerified:
			stats.Verified = count
		case recognitionStatUnregistered:
			stats.Unregistered = count
		}
	}
	return stats, rows.Err()
}
