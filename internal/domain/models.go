package domain

const (
	RoleAdmin   = "admin"
	RoleTeacher = "teacher"

	AttendanceOnTime  = "on_time"
	AttendanceLate    = "late"
	AttendanceLeave   = "leave"
	AttendanceAbsent  = "absent"
	AttendancePending = "pending"
)

type User struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

type Class struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type Student struct {
	ID          int64  `json:"id"`
	StudentNo   string `json:"student_no"`
	Name        string `json:"name"`
	ClassID     int64  `json:"class_id"`
	ClassName   string `json:"class_name,omitempty"`
	FaceSubject string `json:"face_subject"`
	Active      bool   `json:"active"`
}

type Course struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	TeacherID   int64  `json:"teacher_id"`
	TeacherName string `json:"teacher_name,omitempty"`
}

type Schedule struct {
	ID                   int64  `json:"id"`
	CourseID             int64  `json:"course_id"`
	CourseName           string `json:"course_name,omitempty"`
	Classroom            string `json:"classroom"`
	Weekday              int    `json:"weekday"`
	StartMinute          int    `json:"start_minute"`
	EndMinute            int    `json:"end_minute"`
	GraceMinutes         int    `json:"grace_minutes"`
	CheckinBeforeMinutes int    `json:"checkin_before_minutes"`
	Enabled              bool   `json:"enabled"`
}

type AttendanceSession struct {
	ID          int64  `json:"id"`
	ScheduleID  int64  `json:"schedule_id"`
	CourseID    int64  `json:"course_id"`
	CourseName  string `json:"course_name"`
	Classroom   string `json:"classroom"`
	SessionDate string `json:"session_date"`
	StartAt     int64  `json:"start_at"`
	EndAt       int64  `json:"end_at"`
	State       string `json:"state"`
}

type AttendanceRecord struct {
	ID           int64   `json:"id"`
	SessionID    int64   `json:"session_id"`
	StudentID    int64   `json:"student_id"`
	StudentNo    string  `json:"student_no"`
	StudentName  string  `json:"student_name"`
	ClassName    string  `json:"class_name"`
	Status       string  `json:"status"`
	RecognizedAt int64   `json:"recognized_at,omitempty"`
	Similarity   float64 `json:"similarity,omitempty"`
	Source       string  `json:"source"`
	Note         string  `json:"note,omitempty"`
}

type SessionSummary struct {
	AttendanceSession
	Enrolled int `json:"enrolled"`
	OnTime   int `json:"on_time"`
	Late     int `json:"late"`
	Leave    int `json:"leave"`
	Absent   int `json:"absent"`
	Pending  int `json:"pending"`
}

type Leave struct {
	ID         int64  `json:"id"`
	StudentID  int64  `json:"student_id"`
	ScheduleID int64  `json:"schedule_id"`
	LeaveDate  string `json:"leave_date"`
	Reason     string `json:"reason"`
	ApprovedBy int64  `json:"approved_by"`
}
