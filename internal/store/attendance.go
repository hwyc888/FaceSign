package store

import (
	"context"
	"strings"
	"time"
)

func (s *Store) MarkAttendance(ctx context.Context, student Student, similarity float64, now time.Time) (Attendance, bool, error) {
	day := now.Format("2006-01-02")
	result, err := s.db.ExecContext(ctx,
		"INSERT OR IGNORE INTO attendance(student_id,day,checked_at,similarity) VALUES(?,?,?,?)",
		student.ID, day, now.Unix(), similarity,
	)
	if err != nil {
		return Attendance{}, false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return Attendance{}, false, err
	}
	var record Attendance
	var checkedAt int64
	err = s.db.QueryRowContext(ctx, `
        SELECT a.id,s.id,s.student_no,s.name,s.class_name,a.day,a.checked_at,a.similarity
        FROM attendance a JOIN students s ON s.id=a.student_id
        WHERE a.student_id=? AND a.day=?`, student.ID, day,
	).Scan(&record.ID, &record.StudentID, &record.StudentNo, &record.Name, &record.ClassName, &record.Day, &checkedAt, &record.Similarity)
	if err != nil {
		return Attendance{}, false, err
	}
	record.CheckedAt = time.Unix(checkedAt, 0).Format("2006-01-02 15:04:05")
	return record, affected == 1, nil
}

func (s *Store) ListAttendance(ctx context.Context, day string) ([]Attendance, error) {
	day = strings.TrimSpace(day)
	if day == "" {
		day = time.Now().Format("2006-01-02")
	}
	rows, err := s.db.QueryContext(ctx, `
        SELECT a.id,s.id,s.student_no,s.name,s.class_name,a.day,a.checked_at,a.similarity
        FROM attendance a JOIN students s ON s.id=a.student_id
        WHERE a.day=? ORDER BY a.checked_at DESC`, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Attendance, 0)
	for rows.Next() {
		var record Attendance
		var checkedAt int64
		if err := rows.Scan(&record.ID, &record.StudentID, &record.StudentNo, &record.Name, &record.ClassName, &record.Day, &checkedAt, &record.Similarity); err != nil {
			return nil, err
		}
		record.CheckedAt = time.Unix(checkedAt, 0).Format("2006-01-02 15:04:05")
		out = append(out, record)
	}
	return out, rows.Err()
}

func (s *Store) CountStudents(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM students").Scan(&n)
	return n, err
}

