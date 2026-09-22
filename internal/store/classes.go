package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	defaultSeatRows    = 6
	defaultSeatsPerRow = 8
)

func normalizeClassName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("班级名称不能为空")
	}
	if len([]rune(name)) > 40 {
		return "", errors.New("班级名称不能超过40个字符")
	}
	return name, nil
}

func normalizeClassLayout(rows, perRow int) (int, int, error) {
	if rows <= 0 || perRow <= 0 {
		return 0, 0, errors.New("排数和每排人数必须大于0")
	}
	if rows > 20 || perRow > 20 {
		return 0, 0, errors.New("排数和每排人数都不能超过20")
	}
	return rows, perRow, nil
}

func normalizeAttendanceTimes(lateAfter, deadline string) (string, string, error) {
	lateAfter = strings.TrimSpace(lateAfter)
	deadline = strings.TrimSpace(deadline)
	parse := func(label, value string) (int, error) {
		if value == "" {
			return -1, nil
		}
		parsed, err := time.Parse("15:04", value)
		if err != nil {
			return 0, fmt.Errorf("%s必须是 HH:MM 格式", label)
		}
		return parsed.Hour()*60 + parsed.Minute(), nil
	}
	lateMinutes, err := parse("迟到时间", lateAfter)
	if err != nil {
		return "", "", err
	}
	deadlineMinutes, err := parse("签到截止时间", deadline)
	if err != nil {
		return "", "", err
	}
	if lateMinutes >= 0 && deadlineMinutes >= 0 && lateMinutes >= deadlineMinutes {
		return "", "", errors.New("迟到时间必须早于签到截止时间")
	}
	return lateAfter, deadline, nil
}

func (s *Store) ListClasses(ctx context.Context) ([]Class, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id,c.name,c.sort_order,c.seat_rows,c.seats_per_row,c.late_after,c.attendance_deadline,COUNT(s.id)
		FROM classes c
		LEFT JOIN students s ON TRIM(s.class_name)=c.name
		GROUP BY c.id,c.name,c.sort_order,c.seat_rows,c.seats_per_row,c.late_after,c.attendance_deadline
		ORDER BY c.sort_order,c.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Class, 0)
	for rows.Next() {
		var item Class
		if err := rows.Scan(
			&item.ID, &item.Name, &item.SortOrder, &item.SeatRows, &item.SeatsPerRow,
			&item.LateAfter, &item.Deadline, &item.StudentCount,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) CreateClass(ctx context.Context, name string) (Class, error) {
	return s.CreateClassWithSettings(ctx, name, defaultSeatRows, defaultSeatsPerRow, "", "")
}

func (s *Store) CreateClassWithLayout(ctx context.Context, name string, seatRows, seatsPerRow int) (Class, error) {
	return s.CreateClassWithSettings(ctx, name, seatRows, seatsPerRow, "", "")
}

func (s *Store) CreateClassWithSettings(ctx context.Context, name string, seatRows, seatsPerRow int, lateAfter, deadline string) (Class, error) {
	name, err := normalizeClassName(name)
	if err != nil {
		return Class{}, err
	}
	if seatRows == 0 {
		seatRows = defaultSeatRows
	}
	if seatsPerRow == 0 {
		seatsPerRow = defaultSeatsPerRow
	}
	seatRows, seatsPerRow, err = normalizeClassLayout(seatRows, seatsPerRow)
	if err != nil {
		return Class{}, err
	}
	lateAfter, deadline, err = normalizeAttendanceTimes(lateAfter, deadline)
	if err != nil {
		return Class{}, err
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO classes(name,sort_order,seat_rows,seats_per_row,late_after,attendance_deadline,created_at)
		VALUES(?, COALESCE((SELECT MAX(sort_order)+1 FROM classes), 1), ?, ?, ?, ?, ?)`,
		name, seatRows, seatsPerRow, lateAfter, deadline, time.Now().Unix(),
	)
	if err != nil {
		if strings.Contains(err.Error(), "classes.name") {
			return Class{}, errors.New("该班级已经存在")
		}
		return Class{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Class{}, err
	}
	return s.classByID(ctx, id)
}

func (s *Store) classByID(ctx context.Context, id int64) (Class, error) {
	var item Class
	err := s.db.QueryRowContext(ctx, `
		SELECT c.id,c.name,c.sort_order,c.seat_rows,c.seats_per_row,c.late_after,c.attendance_deadline,
		       (SELECT COUNT(*) FROM students s WHERE TRIM(s.class_name)=c.name)
		FROM classes c WHERE c.id=?`, id,
	).Scan(
		&item.ID, &item.Name, &item.SortOrder, &item.SeatRows, &item.SeatsPerRow,
		&item.LateAfter, &item.Deadline, &item.StudentCount,
	)
	return item, err
}

func (s *Store) UpdateClassSettings(ctx context.Context, id int64, name string, seatRows, seatsPerRow int, lateAfter, deadline string) (Class, error) {
	name, err := normalizeClassName(name)
	if err != nil {
		return Class{}, err
	}
	seatRows, seatsPerRow, err = normalizeClassLayout(seatRows, seatsPerRow)
	if err != nil {
		return Class{}, err
	}
	lateAfter, deadline, err = normalizeAttendanceTimes(lateAfter, deadline)
	if err != nil {
		return Class{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Class{}, err
	}
	defer tx.Rollback()

	var oldName string
	if err := tx.QueryRowContext(ctx, "SELECT name FROM classes WHERE id=?", id).Scan(&oldName); err != nil {
		return Class{}, err
	}
	var count, maxSeat int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*),COALESCE(MAX(seat_no),0) FROM students WHERE TRIM(class_name)=?", oldName,
	).Scan(&count, &maxSeat); err != nil {
		return Class{}, err
	}
	capacity := seatRows * seatsPerRow
	if count > capacity {
		return Class{}, fmt.Errorf("当前班级有%d名学生，座位容量只有%d，请增加排数或每排人数", count, capacity)
	}
	if maxSeat > capacity {
		return Class{}, fmt.Errorf("已有座位号%d超过新布局容量%d，请先在座位编辑器中调整学生座位", maxSeat, capacity)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE classes
		SET name=?,seat_rows=?,seats_per_row=?,late_after=?,attendance_deadline=?
		WHERE id=?`, name, seatRows, seatsPerRow, lateAfter, deadline, id,
	); err != nil {
		if strings.Contains(err.Error(), "classes.name") {
			return Class{}, errors.New("该班级已经存在")
		}
		return Class{}, err
	}
	if oldName != name {
		if _, err := tx.ExecContext(ctx, "UPDATE students SET class_name=? WHERE TRIM(class_name)=?", name, oldName); err != nil {
			return Class{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Class{}, err
	}
	return s.classByID(ctx, id)
}

func (s *Store) RenameClass(ctx context.Context, id int64, name string) (Class, error) {
	current, err := s.classByID(ctx, id)
	if err != nil {
		return Class{}, err
	}
	return s.UpdateClassSettings(ctx, id, name, current.SeatRows, current.SeatsPerRow, current.LateAfter, current.Deadline)
}

func (s *Store) UpdateClassLayout(ctx context.Context, id int64, seatRows, seatsPerRow int) (Class, error) {
	current, err := s.classByID(ctx, id)
	if err != nil {
		return Class{}, err
	}
	return s.UpdateClassSettings(ctx, id, current.Name, seatRows, seatsPerRow, current.LateAfter, current.Deadline)
}

func (s *Store) AutoArrangeSeats(ctx context.Context, id int64) (int, error) {
	current, err := s.classByID(ctx, id)
	if err != nil {
		return 0, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx,
		"SELECT id FROM students WHERE TRIM(class_name)=? ORDER BY student_no,id", current.Name,
	)
	if err != nil {
		return 0, err
	}
	ids := make([]int64, 0)
	for rows.Next() {
		var studentID int64
		if err := rows.Scan(&studentID); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, studentID)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if len(ids) > current.SeatRows*current.SeatsPerRow {
		return 0, fmt.Errorf("当前班级有%d名学生，但座位容量只有%d", len(ids), current.SeatRows*current.SeatsPerRow)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE students SET seat_no=0 WHERE TRIM(class_name)=?", current.Name); err != nil {
		return 0, err
	}
	for i, studentID := range ids {
		if _, err := tx.ExecContext(ctx, "UPDATE students SET seat_no=? WHERE id=?", i+1, studentID); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(ids), nil
}

func (s *Store) DeleteClass(ctx context.Context, id int64) error {
	var name string
	if err := s.db.QueryRowContext(ctx, "SELECT name FROM classes WHERE id=?", id).Scan(&name); err != nil {
		return err
	}
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM students WHERE TRIM(class_name)=?", name).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("该班级还有%d名学生，不能删除", count)
	}
	result, err := s.db.ExecContext(ctx, "DELETE FROM classes WHERE id=?", id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) MoveClass(ctx context.Context, id int64, direction string) error {
	if direction != "up" && direction != "down" {
		return errors.New("invalid class move direction")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var currentOrder int
	if err := tx.QueryRowContext(ctx, "SELECT sort_order FROM classes WHERE id=?", id).Scan(&currentOrder); err != nil {
		return err
	}
	operator := "<"
	order := "DESC"
	if direction == "down" {
		operator = ">"
		order = "ASC"
	}
	query := fmt.Sprintf("SELECT id,sort_order FROM classes WHERE sort_order %s ? ORDER BY sort_order %s,id %s LIMIT 1", operator, order, order)
	var neighborID int64
	var neighborOrder int
	err = tx.QueryRowContext(ctx, query, currentOrder).Scan(&neighborID, &neighborOrder)
	if errors.Is(err, sql.ErrNoRows) {
		return tx.Commit()
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE classes SET sort_order=? WHERE id=?", neighborOrder, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE classes SET sort_order=? WHERE id=?", currentOrder, neighborID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ClassExists(ctx context.Context, name string) (bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return false, nil
	}
	var n int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM classes WHERE name=?", name).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}
