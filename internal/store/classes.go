package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
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

func (s *Store) ListClasses(ctx context.Context) ([]Class, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id,c.name,c.sort_order,COUNT(s.id)
		FROM classes c
		LEFT JOIN students s ON TRIM(s.class_name)=c.name
		GROUP BY c.id,c.name,c.sort_order
		ORDER BY c.sort_order,c.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Class, 0)
	for rows.Next() {
		var item Class
		if err := rows.Scan(&item.ID, &item.Name, &item.SortOrder, &item.StudentCount); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) CreateClass(ctx context.Context, name string) (Class, error) {
	name, err := normalizeClassName(name)
	if err != nil {
		return Class{}, err
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO classes(name,sort_order,created_at)
		VALUES(?, COALESCE((SELECT MAX(sort_order)+1 FROM classes), 1), ?)`,
		name, time.Now().Unix(),
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
	var item Class
	err = s.db.QueryRowContext(ctx,
		"SELECT id,name,sort_order,0 FROM classes WHERE id=?", id,
	).Scan(&item.ID, &item.Name, &item.SortOrder, &item.StudentCount)
	return item, err
}

func (s *Store) RenameClass(ctx context.Context, id int64, name string) (Class, error) {
	name, err := normalizeClassName(name)
	if err != nil {
		return Class{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Class{}, err
	}
	defer tx.Rollback()

	var oldName string
	var sortOrder int
	if err := tx.QueryRowContext(ctx, "SELECT name,sort_order FROM classes WHERE id=?", id).Scan(&oldName, &sortOrder); err != nil {
		return Class{}, err
	}
	if oldName != name {
		if _, err := tx.ExecContext(ctx, "UPDATE classes SET name=? WHERE id=?", name, id); err != nil {
			if strings.Contains(err.Error(), "classes.name") {
				return Class{}, errors.New("该班级已经存在")
			}
			return Class{}, err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE students SET class_name=? WHERE TRIM(class_name)=?", name, oldName); err != nil {
			return Class{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Class{}, err
	}
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM students WHERE TRIM(class_name)=?", name).Scan(&count); err != nil {
		return Class{}, err
	}
	return Class{ID: id, Name: name, SortOrder: sortOrder, StudentCount: count}, nil
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

