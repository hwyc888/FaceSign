package store

import (
	"context"
	"database/sql"
	"errors"
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

func (s *Store) AttendanceSeatBoard(ctx context.Context, className, day string) (AttendanceBoard, error) {
	className = strings.TrimSpace(className)
	if className == "" {
		return AttendanceBoard{}, errors.New("请选择班级")
	}
	day = strings.TrimSpace(day)
	if day == "" {
		day = time.Now().Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", day); err != nil {
		return AttendanceBoard{}, errors.New("日期格式不正确")
	}

	var class Class
	err := s.db.QueryRowContext(ctx, `
		SELECT c.id,c.name,c.sort_order,c.seat_rows,c.seats_per_row,COUNT(s.id)
		FROM classes c
		LEFT JOIN students s ON TRIM(s.class_name)=c.name
		WHERE c.name=?
		GROUP BY c.id,c.name,c.sort_order,c.seat_rows,c.seats_per_row`, className,
	).Scan(&class.ID, &class.Name, &class.SortOrder, &class.SeatRows, &class.SeatsPerRow, &class.StudentCount)
	if errors.Is(err, sql.ErrNoRows) {
		return AttendanceBoard{}, errors.New("班级不存在")
	}
	if err != nil {
		return AttendanceBoard{}, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id,s.student_no,s.name,s.class_name,s.seat_no,
		       CASE WHEN a.id IS NULL THEN 0 ELSE 1 END,
		       COALESCE(a.checked_at,0),COALESCE(a.similarity,0)
		FROM students s
		LEFT JOIN attendance a ON a.student_id=s.id AND a.day=?
		WHERE TRIM(s.class_name)=?
		ORDER BY CASE WHEN s.seat_no>0 THEN 0 ELSE 1 END,s.seat_no,s.student_no,s.id`,
		day, className,
	)
	if err != nil {
		return AttendanceBoard{}, err
	}
	defer rows.Close()

	board := AttendanceBoard{Day: day, Class: class, Students: make([]SeatAttendance, 0)}
	for rows.Next() {
		var item SeatAttendance
		var signed int
		var checkedAt int64
		if err := rows.Scan(
			&item.StudentID, &item.StudentNo, &item.Name, &item.ClassName, &item.SeatNo,
			&signed, &checkedAt, &item.Similarity,
		); err != nil {
			return AttendanceBoard{}, err
		}
		item.Signed = signed == 1
		if item.Signed {
			board.Signed++
			item.CheckedAt = time.Unix(checkedAt, 0).Format("15:04:05")
		}
		if item.SeatNo <= 0 {
			board.Unassigned++
		}
		board.Students = append(board.Students, item)
	}
	if err := rows.Err(); err != nil {
		return AttendanceBoard{}, err
	}
	board.Total = len(board.Students)
	board.Unsigned = board.Total - board.Signed
	return board, nil
}

func (s *Store) CountStudents(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM students").Scan(&n)
	return n, err
}
