package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (s *Store) init(ctx context.Context) error {
	statements := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=10000",
		"PRAGMA foreign_keys=ON",
		`CREATE TABLE IF NOT EXISTS students (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            student_no TEXT NOT NULL UNIQUE,
            name TEXT NOT NULL,
            class_name TEXT NOT NULL DEFAULT '',
            seat_no INTEGER NOT NULL DEFAULT 0,
            created_at INTEGER NOT NULL
        )`,
		`CREATE TABLE IF NOT EXISTS classes (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            name TEXT NOT NULL UNIQUE,
            sort_order INTEGER NOT NULL,
            seat_rows INTEGER NOT NULL DEFAULT 6,
            seats_per_row INTEGER NOT NULL DEFAULT 8,
            created_at INTEGER NOT NULL
        )`,
		`CREATE TABLE IF NOT EXISTS face_samples (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            student_id INTEGER NOT NULL,
            embedding BLOB NOT NULL,
            label TEXT NOT NULL DEFAULT '',
            created_at INTEGER NOT NULL,
            FOREIGN KEY(student_id) REFERENCES students(id) ON DELETE CASCADE
        )`,
		`CREATE TABLE IF NOT EXISTS attendance (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            student_id INTEGER NOT NULL,
            day TEXT NOT NULL,
            checked_at INTEGER NOT NULL,
            similarity REAL NOT NULL,
            FOREIGN KEY(student_id) REFERENCES students(id) ON DELETE CASCADE,
            UNIQUE(student_id, day)
        )`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialize database: %w", err)
		}
	}
	if err := s.ensureColumn(ctx, "students", "seat_no", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "classes", "seat_rows", "INTEGER NOT NULL DEFAULT 6"); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "classes", "seats_per_row", "INTEGER NOT NULL DEFAULT 8"); err != nil {
		return err
	}
	if err := s.migrateFaceSamples(ctx); err != nil {
		return err
	}
	if err := s.syncExistingClasses(ctx); err != nil {
		return err
	}
	for _, statement := range []string{
		"CREATE INDEX IF NOT EXISTS idx_face_samples_student ON face_samples(student_id, created_at DESC)",
		"CREATE INDEX IF NOT EXISTS idx_attendance_day ON attendance(day, checked_at DESC)",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_students_class_seat ON students(class_name,seat_no) WHERE seat_no > 0",
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialize database index: %w", err)
		}
	}
	return nil
}


func (s *Store) ensureColumn(ctx context.Context, table, column, definition string) error {
	rows, err := s.db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, pk int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if strings.EqualFold(name, column) {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, "ALTER TABLE "+table+" ADD COLUMN "+column+" "+definition); err != nil {
		return fmt.Errorf("add %s.%s: %w", table, column, err)
	}
	return nil
}

func (s *Store) migrateFaceSamples(ctx context.Context) error {
	var schema string
	if err := s.db.QueryRowContext(ctx,
		"SELECT sql FROM sqlite_master WHERE type='table' AND name='face_samples'",
	).Scan(&schema); err != nil {
		return fmt.Errorf("inspect face_samples: %w", err)
	}

	upper := strings.ToUpper(schema)
	if strings.Contains(upper, "UNIQUE") {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()

		if _, err := tx.ExecContext(ctx, `CREATE TABLE face_samples_migrated (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            student_id INTEGER NOT NULL,
            embedding BLOB NOT NULL,
            label TEXT NOT NULL DEFAULT '',
            created_at INTEGER NOT NULL,
            FOREIGN KEY(student_id) REFERENCES students(id) ON DELETE CASCADE
        )`); err != nil {
			return fmt.Errorf("create migrated face_samples: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO face_samples_migrated(id,student_id,embedding,label,created_at) SELECT id,student_id,embedding,'',created_at FROM face_samples",
		); err != nil {
			return fmt.Errorf("copy face_samples: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "DROP TABLE face_samples"); err != nil {
			return fmt.Errorf("replace face_samples: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "ALTER TABLE face_samples_migrated RENAME TO face_samples"); err != nil {
			return fmt.Errorf("rename face_samples: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		return nil
	}

	rows, err := s.db.QueryContext(ctx, "PRAGMA table_info(face_samples)")
	if err != nil {
		return err
	}
	hasLabel := false
	for rows.Next() {
		var cid, notNull, pk int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			rows.Close()
			return err
		}
		if strings.EqualFold(name, "label") {
			hasLabel = true
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if !hasLabel {
		if _, err := s.db.ExecContext(ctx, "ALTER TABLE face_samples ADD COLUMN label TEXT NOT NULL DEFAULT ''"); err != nil {
			return fmt.Errorf("add face sample label: %w", err)
		}
	}
	return nil
}

func (s *Store) syncExistingClasses(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, "SELECT DISTINCT TRIM(class_name) FROM students WHERE TRIM(class_name) <> '' ORDER BY TRIM(class_name)")
	if err != nil {
		return fmt.Errorf("list existing student classes: %w", err)
	}
	defer rows.Close()

	names := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for _, name := range names {
		if _, err := s.db.ExecContext(ctx, `
			INSERT OR IGNORE INTO classes(name,sort_order,created_at)
			VALUES(?, COALESCE((SELECT MAX(sort_order)+1 FROM classes), 1), ?)`,
			name, time.Now().Unix(),
		); err != nil {
			return fmt.Errorf("sync class %q: %w", name, err)
		}
	}
	return nil
}

