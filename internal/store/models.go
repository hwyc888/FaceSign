package store

import "database/sql"

type Student struct {
	ID        int64  `json:"id"`
	StudentNo string `json:"student_no"`
	Name      string `json:"name"`
	ClassName string `json:"class_name"`
	HasFace   bool   `json:"has_face"`
	FaceCount int    `json:"face_count"`
}

type Class struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	SortOrder    int    `json:"sort_order"`
	StudentCount int    `json:"student_count"`
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

type Store struct { db *sql.DB }
