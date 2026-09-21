package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

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

