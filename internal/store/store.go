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

	_ "modernc.org/sqlite"
)

type Student struct {
	ID        int64  `json:"id"`
	StudentNo string `json:"student_no"`
	Name      string `json:"name"`
	ClassName string `json:"class_name"`
	HasFace   bool   `json:"has_face"`
}

type FaceSample struct {
	Student   Student
	Embedding []byte
}

type Attendance struct {
	ID         int64   `json:"id"`
	StudentID  int64   `json:"student_id"`
	StudentNo  string  `json:"student_no"`
	Name       string  `json:"name"`
	ClassName  string  `json:"class_name"`
	Day        string  `json:"day"`
	CheckedAt  string  `json:"checked_at"`
	Similarity float64 `json:"similarity"`
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("database path is empty")
	}
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create data directory: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.init(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

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
            created_at INTEGER NOT NULL
        )`,
		`CREATE TABLE IF NOT EXISTS face_samples (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            student_id INTEGER NOT NULL UNIQUE,
            embedding BLOB NOT NULL,
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
		"CREATE INDEX IF NOT EXISTS idx_attendance_day ON attendance(day, checked_at DESC)",
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialize database: %w", err)
		}
	}
	return nil
}

func (s *Store) CreateStudent(ctx context.Context, studentNo, name, className string) (Student, error) {
	studentNo = strings.TrimSpace(studentNo)
	name = strings.TrimSpace(name)
	className = strings.TrimSpace(className)
	if studentNo == "" || name == "" {
		return Student{}, errors.New("student number and name are required")
	}
	result, err := s.db.ExecContext(ctx,
		"INSERT INTO students(student_no,name,class_name,created_at) VALUES(?,?,?,?)",
		studentNo, name, className, time.Now().Unix(),
	)
	if err != nil {
		return Student{}, fmt.Errorf("create student: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Student{}, err
	}
	return Student{ID: id, StudentNo: studentNo, Name: name, ClassName: className}, nil
}

func (s *Store) ListStudents(ctx context.Context) ([]Student, error) {
	rows, err := s.db.QueryContext(ctx, `
        SELECT s.id,s.student_no,s.name,s.class_name,
               CASE WHEN f.student_id IS NULL THEN 0 ELSE 1 END
        FROM students s
        LEFT JOIN face_samples f ON f.student_id=s.id
        ORDER BY s.class_name,s.student_no,s.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Student, 0)
	for rows.Next() {
		var student Student
		var hasFace int
		if err := rows.Scan(&student.ID, &student.StudentNo, &student.Name, &student.ClassName, &hasFace); err != nil {
			return nil, err
		}
		student.HasFace = hasFace != 0
		out = append(out, student)
	}
	return out, rows.Err()
}

func (s *Store) DeleteStudent(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM students WHERE id=?", id)
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

func (s *Store) SetFaceSample(ctx context.Context, studentID int64, embedding []byte) error {
	if len(embedding) == 0 {
		return errors.New("empty face embedding")
	}
	_, err := s.db.ExecContext(ctx, `
        INSERT INTO face_samples(student_id,embedding,created_at) VALUES(?,?,?)
        ON CONFLICT(student_id) DO UPDATE SET embedding=excluded.embedding, created_at=excluded.created_at`,
		studentID, embedding, time.Now().Unix(),
	)
	return err
}

func (s *Store) ListFaceSamples(ctx context.Context) ([]FaceSample, error) {
	rows, err := s.db.QueryContext(ctx, `
        SELECT s.id,s.student_no,s.name,s.class_name,f.embedding
        FROM face_samples f
        JOIN students s ON s.id=f.student_id
        ORDER BY s.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]FaceSample, 0)
	for rows.Next() {
		var sample FaceSample
		if err := rows.Scan(&sample.Student.ID, &sample.Student.StudentNo, &sample.Student.Name, &sample.Student.ClassName, &sample.Embedding); err != nil {
			return nil, err
		}
		sample.Student.HasFace = true
		out = append(out, sample)
	}
	return out, rows.Err()
}

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
