package repository

import "context"

func (r *Repository) TeacherCanAccessSession(ctx context.Context, teacherID, sessionID int64) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM attendance_sessions a
		JOIN schedules s ON s.id=a.schedule_id
		JOIN courses c ON c.id=s.course_id
		WHERE a.id=? AND c.teacher_id=?`, sessionID, teacherID).Scan(&count)
	return count > 0, err
}

func (r *Repository) TeacherCanAccessSchedule(ctx context.Context, teacherID, scheduleID int64) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schedules s JOIN courses c ON c.id=s.course_id WHERE s.id=? AND c.teacher_id=?`, scheduleID, teacherID).Scan(&count)
	return count > 0, err
}
