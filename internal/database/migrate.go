package database

import (
	"database/sql"
	"fmt"
)

func migrate(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			display_name TEXT NOT NULL,
			role TEXT NOT NULL CHECK(role IN ('admin','teacher')),
			created_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS login_sessions (
			token_hash TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			expires_at INTEGER NOT NULL,
			created_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_login_sessions_expiry ON login_sessions(expires_at)`,
		`CREATE TABLE IF NOT EXISTS classes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE
		)`,
		`CREATE TABLE IF NOT EXISTS students (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			student_no TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			class_id INTEGER NOT NULL REFERENCES classes(id) ON DELETE RESTRICT,
			face_subject TEXT NOT NULL UNIQUE,
			active INTEGER NOT NULL DEFAULT 1 CHECK(active IN (0,1)),
			created_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_students_class ON students(class_id, active)`,
		`CREATE TABLE IF NOT EXISTS courses (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			teacher_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
			created_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_courses_teacher ON courses(teacher_id)`,
		`CREATE TABLE IF NOT EXISTS course_students (
			course_id INTEGER NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
			student_id INTEGER NOT NULL REFERENCES students(id) ON DELETE CASCADE,
			PRIMARY KEY(course_id, student_id)
		)`,
		`CREATE TABLE IF NOT EXISTS schedules (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			course_id INTEGER NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
			classroom TEXT NOT NULL,
			weekday INTEGER NOT NULL CHECK(weekday BETWEEN 1 AND 7),
			start_minute INTEGER NOT NULL CHECK(start_minute BETWEEN 0 AND 1439),
			end_minute INTEGER NOT NULL CHECK(end_minute BETWEEN 1 AND 1440),
			grace_minutes INTEGER NOT NULL DEFAULT 5 CHECK(grace_minutes BETWEEN 0 AND 120),
			checkin_before_minutes INTEGER NOT NULL DEFAULT 20 CHECK(checkin_before_minutes BETWEEN 0 AND 180),
			enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0,1)),
			CHECK(end_minute > start_minute)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_schedules_lookup ON schedules(weekday, classroom, enabled, start_minute, end_minute)`,
		`CREATE TABLE IF NOT EXISTS attendance_sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			schedule_id INTEGER NOT NULL REFERENCES schedules(id) ON DELETE RESTRICT,
			session_date TEXT NOT NULL,
			start_at INTEGER NOT NULL,
			end_at INTEGER NOT NULL,
			state TEXT NOT NULL DEFAULT 'open' CHECK(state IN ('open','closed')),
			created_at INTEGER NOT NULL,
			UNIQUE(schedule_id, session_date)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_attendance_sessions_state ON attendance_sessions(state, end_at)`,
		`CREATE TABLE IF NOT EXISTS attendance_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id INTEGER NOT NULL REFERENCES attendance_sessions(id) ON DELETE RESTRICT,
			student_id INTEGER NOT NULL REFERENCES students(id) ON DELETE RESTRICT,
			status TEXT NOT NULL CHECK(status IN ('on_time','late','leave','absent')),
			recognized_at INTEGER,
			similarity REAL,
			source TEXT NOT NULL,
			note TEXT NOT NULL DEFAULT '',
			updated_at INTEGER NOT NULL,
			UNIQUE(session_id, student_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_attendance_records_session ON attendance_records(session_id, status)`,
		`CREATE TABLE IF NOT EXISTS leave_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			student_id INTEGER NOT NULL REFERENCES students(id) ON DELETE RESTRICT,
			schedule_id INTEGER NOT NULL REFERENCES schedules(id) ON DELETE RESTRICT,
			leave_date TEXT NOT NULL,
			reason TEXT NOT NULL DEFAULT '',
			approved_by INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
			created_at INTEGER NOT NULL,
			UNIQUE(student_id, schedule_id, leave_date)
		)`,
	}

	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return fmt.Errorf("database migration failed: %w", err)
		}
	}
	return nil
}
