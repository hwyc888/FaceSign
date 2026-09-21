package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

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

