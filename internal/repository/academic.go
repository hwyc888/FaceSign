package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/hwyc888/FaceSign/internal/domain"
)

func (r *Repository) ListClasses(ctx context.Context) ([]domain.Class, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, name FROM classes ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.Class
	for rows.Next() {
		var item domain.Class
		if err := rows.Scan(&item.ID, &item.Name); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) CreateClass(ctx context.Context, name string) (domain.Class, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.Class{}, fmt.Errorf("class name is required")
	}
	result, err := r.db.ExecContext(ctx, `INSERT INTO classes(name) VALUES(?)`, name)
	if err != nil {
		return domain.Class{}, err
	}
	id, err := result.LastInsertId()
	return domain.Class{ID: id, Name: name}, err
}

func (r *Repository) UpdateClass(ctx context.Context, id int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("class name is required")
	}
	result, err := r.db.ExecContext(ctx, `UPDATE classes SET name=? WHERE id=?`, name, id)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (r *Repository) DeleteClass(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM classes WHERE id=?`, id)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (r *Repository) ListStudents(ctx context.Context) ([]domain.Student, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT s.id, s.student_no, s.name, s.class_id, c.name, s.face_subject, s.active
		FROM students s JOIN classes c ON c.id=s.class_id
		ORDER BY c.name, s.student_no, s.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.Student
	for rows.Next() {
		var item domain.Student
		var active int
		if err := rows.Scan(&item.ID, &item.StudentNo, &item.Name, &item.ClassID, &item.ClassName, &item.FaceSubject, &active); err != nil {
			return nil, err
		}
		item.Active = active == 1
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) CreateStudent(ctx context.Context, studentNo, name string, classID int64) (domain.Student, error) {
	studentNo, name = strings.TrimSpace(studentNo), strings.TrimSpace(name)
	if studentNo == "" || name == "" || classID <= 0 {
		return domain.Student{}, fmt.Errorf("student number, name and class are required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Student{}, err
	}
	defer tx.Rollback()
	temporarySubject := "pending_" + studentNo
	result, err := tx.ExecContext(ctx, `INSERT INTO students(student_no, name, class_id, face_subject, active, created_at) VALUES(?,?,?,?,1,?)`, studentNo, name, classID, temporarySubject, time.Now().Unix())
	if err != nil {
		return domain.Student{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return domain.Student{}, err
	}
	subject := fmt.Sprintf("student_%d", id)
	if _, err := tx.ExecContext(ctx, `UPDATE students SET face_subject=? WHERE id=?`, subject, id); err != nil {
		return domain.Student{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Student{}, err
	}
	return domain.Student{ID: id, StudentNo: studentNo, Name: name, ClassID: classID, FaceSubject: subject, Active: true}, nil
}

func (r *Repository) UpdateStudent(ctx context.Context, id int64, studentNo, name string, classID int64, active bool) error {
	studentNo, name = strings.TrimSpace(studentNo), strings.TrimSpace(name)
	if studentNo == "" || name == "" || classID <= 0 {
		return fmt.Errorf("student number, name and class are required")
	}
	activeInt := 0
	if active {
		activeInt = 1
	}
	result, err := r.db.ExecContext(ctx, `UPDATE students SET student_no=?, name=?, class_id=?, active=? WHERE id=?`, studentNo, name, classID, activeInt, id)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (r *Repository) DeleteStudent(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM students WHERE id=?`, id)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (r *Repository) StudentByID(ctx context.Context, id int64) (domain.Student, error) {
	var item domain.Student
	var active int
	err := r.db.QueryRowContext(ctx, `SELECT s.id,s.student_no,s.name,s.class_id,c.name,s.face_subject,s.active FROM students s JOIN classes c ON c.id=s.class_id WHERE s.id=?`, id).
		Scan(&item.ID, &item.StudentNo, &item.Name, &item.ClassID, &item.ClassName, &item.FaceSubject, &active)
	item.Active = active == 1
	return item, err
}

func (r *Repository) StudentByFaceSubject(ctx context.Context, subject string) (domain.Student, error) {
	var item domain.Student
	var active int
	err := r.db.QueryRowContext(ctx, `SELECT s.id,s.student_no,s.name,s.class_id,c.name,s.face_subject,s.active FROM students s JOIN classes c ON c.id=s.class_id WHERE s.face_subject=?`, subject).
		Scan(&item.ID, &item.StudentNo, &item.Name, &item.ClassID, &item.ClassName, &item.FaceSubject, &active)
	item.Active = active == 1
	return item, err
}

func (r *Repository) ListCourses(ctx context.Context) ([]domain.Course, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT c.id,c.name,c.teacher_id,u.display_name FROM courses c JOIN users u ON u.id=c.teacher_id ORDER BY c.name,c.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.Course
	for rows.Next() {
		var item domain.Course
		if err := rows.Scan(&item.ID, &item.Name, &item.TeacherID, &item.TeacherName); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) CreateCourse(ctx context.Context, name string, teacherID int64) (domain.Course, error) {
	name = strings.TrimSpace(name)
	if name == "" || teacherID <= 0 {
		return domain.Course{}, fmt.Errorf("course name and teacher are required")
	}
	result, err := r.db.ExecContext(ctx, `INSERT INTO courses(name,teacher_id,created_at) VALUES(?,?,?)`, name, teacherID, time.Now().Unix())
	if err != nil {
		return domain.Course{}, err
	}
	id, err := result.LastInsertId()
	return domain.Course{ID: id, Name: name, TeacherID: teacherID}, err
}

func (r *Repository) UpdateCourse(ctx context.Context, id int64, name string, teacherID int64) error {
	name = strings.TrimSpace(name)
	if name == "" || teacherID <= 0 {
		return fmt.Errorf("course name and teacher are required")
	}
	result, err := r.db.ExecContext(ctx, `UPDATE courses SET name=?,teacher_id=? WHERE id=?`, name, teacherID, id)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (r *Repository) DeleteCourse(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM courses WHERE id=?`, id)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (r *Repository) AddStudentToCourse(ctx context.Context, courseID, studentID int64) error {
	_, err := r.db.ExecContext(ctx, `INSERT OR IGNORE INTO course_students(course_id,student_id) VALUES(?,?)`, courseID, studentID)
	return err
}

func (r *Repository) RemoveStudentFromCourse(ctx context.Context, courseID, studentID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM course_students WHERE course_id=? AND student_id=?`, courseID, studentID)
	return err
}

func (r *Repository) CourseStudents(ctx context.Context, courseID int64) ([]domain.Student, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT s.id,s.student_no,s.name,s.class_id,c.name,s.face_subject,s.active FROM course_students cs JOIN students s ON s.id=cs.student_id JOIN classes c ON c.id=s.class_id WHERE cs.course_id=? ORDER BY c.name,s.student_no`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.Student
	for rows.Next() {
		var item domain.Student
		var active int
		if err := rows.Scan(&item.ID, &item.StudentNo, &item.Name, &item.ClassID, &item.ClassName, &item.FaceSubject, &active); err != nil {
			return nil, err
		}
		item.Active = active == 1
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) ListSchedules(ctx context.Context) ([]domain.Schedule, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT s.id,s.course_id,c.name,s.classroom,s.weekday,s.start_minute,s.end_minute,s.grace_minutes,s.checkin_before_minutes,s.enabled FROM schedules s JOIN courses c ON c.id=s.course_id ORDER BY s.weekday,s.start_minute,s.classroom`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.Schedule
	for rows.Next() {
		var item domain.Schedule
		var enabled int
		if err := rows.Scan(&item.ID, &item.CourseID, &item.CourseName, &item.Classroom, &item.Weekday, &item.StartMinute, &item.EndMinute, &item.GraceMinutes, &item.CheckinBeforeMinutes, &enabled); err != nil {
			return nil, err
		}
		item.Enabled = enabled == 1
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) CreateSchedule(ctx context.Context, item domain.Schedule) (domain.Schedule, error) {
	if err := validateSchedule(item); err != nil {
		return domain.Schedule{}, err
	}
	result, err := r.db.ExecContext(ctx, `INSERT INTO schedules(course_id,classroom,weekday,start_minute,end_minute,grace_minutes,checkin_before_minutes,enabled) VALUES(?,?,?,?,?,?,?,?)`, item.CourseID, strings.TrimSpace(item.Classroom), item.Weekday, item.StartMinute, item.EndMinute, item.GraceMinutes, item.CheckinBeforeMinutes, boolInt(item.Enabled))
	if err != nil {
		return domain.Schedule{}, err
	}
	item.ID, err = result.LastInsertId()
	return item, err
}

func (r *Repository) UpdateSchedule(ctx context.Context, item domain.Schedule) error {
	if item.ID <= 0 {
		return fmt.Errorf("schedule id is required")
	}
	if err := validateSchedule(item); err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `UPDATE schedules SET course_id=?,classroom=?,weekday=?,start_minute=?,end_minute=?,grace_minutes=?,checkin_before_minutes=?,enabled=? WHERE id=?`, item.CourseID, strings.TrimSpace(item.Classroom), item.Weekday, item.StartMinute, item.EndMinute, item.GraceMinutes, item.CheckinBeforeMinutes, boolInt(item.Enabled), item.ID)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (r *Repository) DeleteSchedule(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM schedules WHERE id=?`, id)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func validateSchedule(item domain.Schedule) error {
	if item.CourseID <= 0 || strings.TrimSpace(item.Classroom) == "" {
		return fmt.Errorf("course and classroom are required")
	}
	if item.Weekday < 1 || item.Weekday > 7 {
		return fmt.Errorf("weekday must be between 1 and 7")
	}
	if item.StartMinute < 0 || item.StartMinute > 1439 || item.EndMinute < 1 || item.EndMinute > 1440 || item.EndMinute <= item.StartMinute {
		return fmt.Errorf("invalid class time")
	}
	if item.GraceMinutes < 0 || item.GraceMinutes > 120 {
		return fmt.Errorf("grace minutes must be between 0 and 120")
	}
	if item.CheckinBeforeMinutes < 0 || item.CheckinBeforeMinutes > 180 {
		return fmt.Errorf("check-in lead time must be between 0 and 180")
	}
	return nil
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func requireAffected(result sql.Result) error {
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
