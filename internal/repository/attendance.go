package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/hwyc888/FaceSign/internal/domain"
)

func (r *Repository) FindActiveSchedule(ctx context.Context, classroom string, weekday, minute int) (domain.Schedule, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT s.id,s.course_id,c.name,s.classroom,s.weekday,s.start_minute,s.end_minute,s.grace_minutes,s.checkin_before_minutes,s.enabled
		FROM schedules s JOIN courses c ON c.id=s.course_id
		WHERE s.enabled=1 AND s.classroom=? AND s.weekday=?
		  AND ? >= (s.start_minute-s.checkin_before_minutes) AND ? <= s.end_minute
		ORDER BY s.start_minute
		LIMIT 2`, classroom, weekday, minute, minute)
	if err != nil {
		return domain.Schedule{}, err
	}
	defer rows.Close()
	var found []domain.Schedule
	for rows.Next() {
		var item domain.Schedule
		var enabled int
		if err := rows.Scan(&item.ID, &item.CourseID, &item.CourseName, &item.Classroom, &item.Weekday, &item.StartMinute, &item.EndMinute, &item.GraceMinutes, &item.CheckinBeforeMinutes, &enabled); err != nil {
			return domain.Schedule{}, err
		}
		item.Enabled = enabled == 1
		found = append(found, item)
	}
	if err := rows.Err(); err != nil {
		return domain.Schedule{}, err
	}
	if len(found) == 0 {
		return domain.Schedule{}, sql.ErrNoRows
	}
	if len(found) > 1 {
		return domain.Schedule{}, fmt.Errorf("multiple overlapping schedules found for classroom %s", classroom)
	}
	return found[0], nil
}

func (r *Repository) SchedulesForWeekday(ctx context.Context, weekday int) ([]domain.Schedule, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT s.id,s.course_id,c.name,s.classroom,s.weekday,s.start_minute,s.end_minute,s.grace_minutes,s.checkin_before_minutes,s.enabled
		FROM schedules s JOIN courses c ON c.id=s.course_id
		WHERE s.enabled=1 AND s.weekday=? ORDER BY s.start_minute`, weekday)
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

func (r *Repository) EnsureAttendanceSession(ctx context.Context, schedule domain.Schedule, date string, startAt, endAt time.Time) (domain.AttendanceSession, bool, error) {
	result, err := r.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO attendance_sessions(schedule_id,session_date,start_at,end_at,state,created_at)
		VALUES(?,?,?,?,'open',?)`, schedule.ID, date, startAt.Unix(), endAt.Unix(), time.Now().Unix())
	if err != nil {
		return domain.AttendanceSession{}, false, err
	}
	rows, _ := result.RowsAffected()
	var item domain.AttendanceSession
	err = r.db.QueryRowContext(ctx, `
		SELECT a.id,a.schedule_id,s.course_id,c.name,s.classroom,a.session_date,a.start_at,a.end_at,a.state
		FROM attendance_sessions a JOIN schedules s ON s.id=a.schedule_id JOIN courses c ON c.id=s.course_id
		WHERE a.schedule_id=? AND a.session_date=?`, schedule.ID, date).
		Scan(&item.ID, &item.ScheduleID, &item.CourseID, &item.CourseName, &item.Classroom, &item.SessionDate, &item.StartAt, &item.EndAt, &item.State)
	return item, rows > 0, err
}

func (r *Repository) SessionByID(ctx context.Context, id int64) (domain.AttendanceSession, error) {
	var item domain.AttendanceSession
	err := r.db.QueryRowContext(ctx, `
		SELECT a.id,a.schedule_id,s.course_id,c.name,s.classroom,a.session_date,a.start_at,a.end_at,a.state
		FROM attendance_sessions a JOIN schedules s ON s.id=a.schedule_id JOIN courses c ON c.id=s.course_id
		WHERE a.id=?`, id).
		Scan(&item.ID, &item.ScheduleID, &item.CourseID, &item.CourseName, &item.Classroom, &item.SessionDate, &item.StartAt, &item.EndAt, &item.State)
	return item, err
}

func (r *Repository) StudentEnrolled(ctx context.Context, courseID, studentID int64) (bool, error) {
	var one int
	err := r.db.QueryRowContext(ctx, `SELECT 1 FROM course_students cs JOIN students s ON s.id=cs.student_id WHERE cs.course_id=? AND cs.student_id=? AND s.active=1`, courseID, studentID).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (r *Repository) RecordRecognition(ctx context.Context, sessionID, studentID int64, status string, recognizedAt time.Time, similarity float64) (domain.AttendanceRecord, error) {
	if status != domain.AttendanceOnTime && status != domain.AttendanceLate {
		return domain.AttendanceRecord{}, fmt.Errorf("invalid recognition attendance status")
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO attendance_records(session_id,student_id,status,recognized_at,similarity,source,note,updated_at)
		VALUES(?,?,?,?,?,'face','',?)
		ON CONFLICT(session_id,student_id) DO UPDATE SET
		 status=excluded.status, recognized_at=excluded.recognized_at, similarity=excluded.similarity, source='face', note='', updated_at=excluded.updated_at`,
		sessionID, studentID, status, recognizedAt.Unix(), similarity, time.Now().Unix())
	if err != nil {
		return domain.AttendanceRecord{}, err
	}
	return r.AttendanceRecord(ctx, sessionID, studentID)
}

func (r *Repository) RecordManual(ctx context.Context, sessionID, studentID int64, status, note string, at time.Time) (domain.AttendanceRecord, error) {
	switch status {
	case domain.AttendanceOnTime, domain.AttendanceLate, domain.AttendanceLeave, domain.AttendanceAbsent:
	default:
		return domain.AttendanceRecord{}, fmt.Errorf("invalid attendance status")
	}
	var recognized any
	if status == domain.AttendanceOnTime || status == domain.AttendanceLate {
		recognized = at.Unix()
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO attendance_records(session_id,student_id,status,recognized_at,similarity,source,note,updated_at)
		VALUES(?,?,?,?,NULL,'manual',?,?)
		ON CONFLICT(session_id,student_id) DO UPDATE SET status=excluded.status,recognized_at=excluded.recognized_at,similarity=NULL,source='manual',note=excluded.note,updated_at=excluded.updated_at`,
		sessionID, studentID, status, recognized, note, time.Now().Unix())
	if err != nil {
		return domain.AttendanceRecord{}, err
	}
	return r.AttendanceRecord(ctx, sessionID, studentID)
}

func (r *Repository) AttendanceRecord(ctx context.Context, sessionID, studentID int64) (domain.AttendanceRecord, error) {
	var item domain.AttendanceRecord
	var recognized sql.NullInt64
	var similarity sql.NullFloat64
	err := r.db.QueryRowContext(ctx, `
		SELECT ar.id,ar.session_id,s.id,s.student_no,s.name,c.name,ar.status,ar.recognized_at,ar.similarity,ar.source,ar.note
		FROM attendance_records ar JOIN students s ON s.id=ar.student_id JOIN classes c ON c.id=s.class_id
		WHERE ar.session_id=? AND ar.student_id=?`, sessionID, studentID).
		Scan(&item.ID, &item.SessionID, &item.StudentID, &item.StudentNo, &item.StudentName, &item.ClassName, &item.Status, &recognized, &similarity, &item.Source, &item.Note)
	if recognized.Valid {
		item.RecognizedAt = recognized.Int64
	}
	if similarity.Valid {
		item.Similarity = similarity.Float64
	}
	return item, err
}

func (r *Repository) CreateLeave(ctx context.Context, item domain.Leave) (domain.Leave, error) {
	if item.StudentID <= 0 || item.ScheduleID <= 0 || item.LeaveDate == "" || item.ApprovedBy <= 0 {
		return domain.Leave{}, fmt.Errorf("student, schedule, leave date and approver are required")
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO leave_records(student_id,schedule_id,leave_date,reason,approved_by,created_at) VALUES(?,?,?,?,?,?) ON CONFLICT(student_id,schedule_id,leave_date) DO UPDATE SET reason=excluded.reason,approved_by=excluded.approved_by`, item.StudentID, item.ScheduleID, item.LeaveDate, item.Reason, item.ApprovedBy, time.Now().Unix())
	if err != nil {
		return domain.Leave{}, err
	}
	if err := r.db.QueryRowContext(ctx, `SELECT id FROM leave_records WHERE student_id=? AND schedule_id=? AND leave_date=?`, item.StudentID, item.ScheduleID, item.LeaveDate).Scan(&item.ID); err != nil {
		return domain.Leave{}, err
	}
	return item, nil
}

func (r *Repository) ApplyLeavesToSession(ctx context.Context, session domain.AttendanceSession) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO attendance_records(session_id,student_id,status,recognized_at,similarity,source,note,updated_at)
		SELECT ?,lr.student_id,'leave',NULL,NULL,'leave',lr.reason,?
		FROM leave_records lr JOIN course_students cs ON cs.student_id=lr.student_id
		WHERE lr.schedule_id=? AND lr.leave_date=? AND cs.course_id=?
		ON CONFLICT(session_id,student_id) DO UPDATE SET
		 status='leave', recognized_at=NULL, similarity=NULL, source='leave', note=excluded.note, updated_at=excluded.updated_at
		 WHERE attendance_records.status='absent'`, session.ID, time.Now().Unix(), session.ScheduleID, session.SessionDate, session.CourseID)
	return err
}

func (r *Repository) FinalizeSession(ctx context.Context, session domain.AttendanceSession) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO attendance_records(session_id,student_id,status,recognized_at,similarity,source,note,updated_at)
		SELECT ?,s.id,'absent',NULL,NULL,'system','',?
		FROM course_students cs JOIN students s ON s.id=cs.student_id
		WHERE cs.course_id=? AND s.active=1
		ON CONFLICT(session_id,student_id) DO NOTHING`, session.ID, time.Now().Unix(), session.CourseID)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE attendance_sessions SET state='closed' WHERE id=?`, session.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) SessionsDueForFinalization(ctx context.Context, now time.Time) ([]domain.AttendanceSession, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT a.id,a.schedule_id,s.course_id,c.name,s.classroom,a.session_date,a.start_at,a.end_at,a.state
		FROM attendance_sessions a JOIN schedules s ON s.id=a.schedule_id JOIN courses c ON c.id=s.course_id
		WHERE a.state='open' AND a.end_at<=? ORDER BY a.end_at`, now.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.AttendanceSession
	for rows.Next() {
		var item domain.AttendanceSession
		if err := rows.Scan(&item.ID, &item.ScheduleID, &item.CourseID, &item.CourseName, &item.Classroom, &item.SessionDate, &item.StartAt, &item.EndAt, &item.State); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) DashboardSummaries(ctx context.Context, date string, teacherID int64, now time.Time) ([]domain.SessionSummary, error) {
	query := `
		SELECT a.id,a.schedule_id,s.course_id,c.name,s.classroom,a.session_date,a.start_at,a.end_at,a.state
		FROM attendance_sessions a JOIN schedules s ON s.id=a.schedule_id JOIN courses c ON c.id=s.course_id
		WHERE a.session_date=? AND (a.state='open' OR a.end_at>=?)`
	args := []any{date, now.Add(-time.Hour).Unix()}
	if teacherID > 0 {
		query += ` AND c.teacher_id=?`
		args = append(args, teacherID)
	}
	query += ` ORDER BY a.start_at,s.classroom`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.SessionSummary
	for rows.Next() {
		var summary domain.SessionSummary
		if err := rows.Scan(&summary.ID, &summary.ScheduleID, &summary.CourseID, &summary.CourseName, &summary.Classroom, &summary.SessionDate, &summary.StartAt, &summary.EndAt, &summary.State); err != nil {
			return nil, err
		}
		items = append(items, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range items {
		if err := r.fillSummaryCounts(ctx, &items[i]); err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (r *Repository) fillSummaryCounts(ctx context.Context, summary *domain.SessionSummary) error {
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM course_students cs JOIN students s ON s.id=cs.student_id WHERE cs.course_id=? AND s.active=1`, summary.CourseID).Scan(&summary.Enrolled); err != nil {
		return err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT status,COUNT(*) FROM attendance_records WHERE session_id=? GROUP BY status`, summary.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return err
		}
		switch status {
		case domain.AttendanceOnTime:
			summary.OnTime = count
		case domain.AttendanceLate:
			summary.Late = count
		case domain.AttendanceLeave:
			summary.Leave = count
		case domain.AttendanceAbsent:
			summary.Absent = count
		}
	}
	summary.Pending = summary.Enrolled - summary.OnTime - summary.Late - summary.Leave - summary.Absent
	if summary.Pending < 0 {
		summary.Pending = 0
	}
	return rows.Err()
}

func (r *Repository) SessionRecords(ctx context.Context, sessionID int64) ([]domain.AttendanceRecord, error) {
	session, err := r.SessionByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT s.id,s.student_no,s.name,c.name,COALESCE(ar.id,0),COALESCE(ar.status,'pending'),ar.recognized_at,ar.similarity,COALESCE(ar.source,''),COALESCE(ar.note,'')
		FROM course_students cs JOIN students s ON s.id=cs.student_id JOIN classes c ON c.id=s.class_id
		LEFT JOIN attendance_records ar ON ar.session_id=? AND ar.student_id=s.id
		WHERE cs.course_id=? ORDER BY c.name,s.student_no,s.id`, sessionID, session.CourseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.AttendanceRecord
	for rows.Next() {
		var item domain.AttendanceRecord
		var recognized sql.NullInt64
		var similarity sql.NullFloat64
		if err := rows.Scan(&item.StudentID, &item.StudentNo, &item.StudentName, &item.ClassName, &item.ID, &item.Status, &recognized, &similarity, &item.Source, &item.Note); err != nil {
			return nil, err
		}
		item.SessionID = sessionID
		if recognized.Valid {
			item.RecognizedAt = recognized.Int64
		}
		if similarity.Valid {
			item.Similarity = similarity.Float64
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
