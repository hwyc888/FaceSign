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
	FaceCount int    `json:"face_count"`
}

type FaceSample struct {
	ID        int64
	Student   Student
	Embedding []byte
	Label     string
	CreatedAt int64
}

type FaceSampleInfo struct {
	ID        int64  `json:"id"`
	StudentID int64  `json:"student_id"`
	Label     string `json:"label"`
	CreatedAt string `json:"created_at"`
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
	if err := s.migrateFaceSamples(ctx); err != nil {
		return err
	}
	for _, statement := range []string{
		"CREATE INDEX IF NOT EXISTS idx_face_samples_student ON face_samples(student_id, created_at DESC)",
		"CREATE INDEX IF NOT EXISTS idx_attendance_day ON attendance(day, checked_at DESC)",
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialize database index: %w", err)
		}
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

func (s *Store) CreateStudentWithFace(ctx context.Context, studentNo, name, className, label string, embedding []byte) (Student, FaceSampleInfo, error) {
	studentNo = strings.TrimSpace(studentNo)
	name = strings.TrimSpace(name)
	className = strings.TrimSpace(className)
	label = normalizeFaceLabel(label)
	if studentNo == "" || name == "" {
		return Student{}, FaceSampleInfo{}, errors.New("student number and name are required")
	}
	if len(embedding) == 0 {
		return Student{}, FaceSampleInfo{}, errors.New("empty face embedding")
	}

	now := time.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Student{}, FaceSampleInfo{}, err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx,
		"INSERT INTO students(student_no,name,class_name,created_at) VALUES(?,?,?,?)",
		studentNo, name, className, now.Unix(),
	)
	if err != nil {
		return Student{}, FaceSampleInfo{}, fmt.Errorf("create student: %w", err)
	}
	studentID, err := result.LastInsertId()
	if err != nil {
		return Student{}, FaceSampleInfo{}, err
	}
	sampleResult, err := tx.ExecContext(ctx,
		"INSERT INTO face_samples(student_id,embedding,label,created_at) VALUES(?,?,?,?)",
		studentID, embedding, label, now.Unix(),
	)
	if err != nil {
		return Student{}, FaceSampleInfo{}, fmt.Errorf("create face sample: %w", err)
	}
	sampleID, err := sampleResult.LastInsertId()
	if err != nil {
		return Student{}, FaceSampleInfo{}, err
	}
	if err := tx.Commit(); err != nil {
		return Student{}, FaceSampleInfo{}, err
	}

	student := Student{
		ID:        studentID,
		StudentNo: studentNo,
		Name:      name,
		ClassName: className,
		HasFace:   true,
		FaceCount: 1,
	}
	info := FaceSampleInfo{
		ID:        sampleID,
		StudentID: studentID,
		Label:     label,
		CreatedAt: now.Format("2006-01-02 15:04:05"),
	}
	return student, info, nil
}

func (s *Store) ListStudents(ctx context.Context) ([]Student, error) {
	rows, err := s.db.QueryContext(ctx, `
        SELECT s.id,s.student_no,s.name,s.class_name,COUNT(f.id)
        FROM students s
        LEFT JOIN face_samples f ON f.student_id=s.id
        GROUP BY s.id,s.student_no,s.name,s.class_name
        ORDER BY s.class_name,s.student_no,s.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Student, 0)
	for rows.Next() {
		var student Student
		if err := rows.Scan(&student.ID, &student.StudentNo, &student.Name, &student.ClassName, &student.FaceCount); err != nil {
			return nil, err
		}
		student.HasFace = student.FaceCount > 0
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

// SetFaceSample remains for compatibility with older API callers. It now adds
// a sample instead of replacing the student's previous face sample.
func (s *Store) SetFaceSample(ctx context.Context, studentID int64, embedding []byte) error {
	_, err := s.AddFaceSample(ctx, studentID, "补充", embedding)
	return err
}

func (s *Store) AddFaceSample(ctx context.Context, studentID int64, label string, embedding []byte) (FaceSampleInfo, error) {
	if studentID <= 0 {
		return FaceSampleInfo{}, errors.New("invalid student id")
	}
	if len(embedding) == 0 {
		return FaceSampleInfo{}, errors.New("empty face embedding")
	}
	label = normalizeFaceLabel(label)
	now := time.Now()
	result, err := s.db.ExecContext(ctx,
		"INSERT INTO face_samples(student_id,embedding,label,created_at) VALUES(?,?,?,?)",
		studentID, embedding, label, now.Unix(),
	)
	if err != nil {
		return FaceSampleInfo{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return FaceSampleInfo{}, err
	}
	return FaceSampleInfo{
		ID:        id,
		StudentID: studentID,
		Label:     label,
		CreatedAt: now.Format("2006-01-02 15:04:05"),
	}, nil
}

func normalizeFaceLabel(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return "补充"
	}
	if len([]rune(label)) > 20 {
		return string([]rune(label)[:20])
	}
	return label
}

func (s *Store) ListFaceSamples(ctx context.Context) ([]FaceSample, error) {
	rows, err := s.db.QueryContext(ctx, `
        SELECT f.id,s.id,s.student_no,s.name,s.class_name,f.embedding,f.label,f.created_at
        FROM face_samples f
        JOIN students s ON s.id=f.student_id
        ORDER BY s.id,f.created_at,f.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]FaceSample, 0)
	for rows.Next() {
		var sample FaceSample
		if err := rows.Scan(
			&sample.ID,
			&sample.Student.ID,
			&sample.Student.StudentNo,
			&sample.Student.Name,
			&sample.Student.ClassName,
			&sample.Embedding,
			&sample.Label,
			&sample.CreatedAt,
		); err != nil {
			return nil, err
		}
		sample.Student.HasFace = true
		out = append(out, sample)
	}
	return out, rows.Err()
}

func (s *Store) ListStudentFaceSamples(ctx context.Context, studentID int64) ([]FaceSampleInfo, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id,student_id,label,created_at FROM face_samples WHERE student_id=? ORDER BY created_at DESC,id DESC",
		studentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]FaceSampleInfo, 0)
	for rows.Next() {
		var info FaceSampleInfo
		var createdAt int64
		if err := rows.Scan(&info.ID, &info.StudentID, &info.Label, &createdAt); err != nil {
			return nil, err
		}
		info.CreatedAt = time.Unix(createdAt, 0).Format("2006-01-02 15:04:05")
		out = append(out, info)
	}
	return out, rows.Err()
}

func (s *Store) DeleteFaceSample(ctx context.Context, studentID, sampleID int64) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM face_samples WHERE id=? AND student_id=?", sampleID, studentID)
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
