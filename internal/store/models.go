package store

import "database/sql"

type Student struct {
	ID        int64  `json:"id"`
	StudentNo string `json:"student_no"`
	Name      string `json:"name"`
	ClassName string `json:"class_name"`
	SeatNo    int    `json:"seat_no"`
	HasFace   bool   `json:"has_face"`
	FaceCount int    `json:"face_count"`
}

type Class struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	SortOrder    int    `json:"sort_order"`
	SeatRows     int    `json:"seat_rows"`
	SeatsPerRow  int    `json:"seats_per_row"`
	LateAfter    string `json:"late_after"`
	Deadline     string `json:"deadline"`
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

type SeatAttendance struct {
	StudentID  int64   `json:"student_id"`
	StudentNo  string  `json:"student_no"`
	Name       string  `json:"name"`
	ClassName  string  `json:"class_name"`
	SeatNo     int     `json:"seat_no"`
	Signed     bool    `json:"signed"`
	Status     string  `json:"status"`
	CheckedAt  string  `json:"checked_at,omitempty"`
	Similarity float64 `json:"similarity,omitempty"`
}

type AttendanceBoard struct {
	Day        string           `json:"day"`
	Class      Class            `json:"class"`
	Total      int              `json:"total"`
	Signed     int              `json:"signed"`
	OnTime     int              `json:"on_time"`
	Late       int              `json:"late"`
	Unsigned   int              `json:"unsigned"`
	Waiting    int              `json:"waiting"`
	Absent     int              `json:"absent"`
	EmptySeats int              `json:"empty_seats"`
	Unassigned int              `json:"unassigned"`
	Students   []SeatAttendance `json:"students"`
}

type SeatMoveResult struct {
	MovedStudentID  int64 `json:"moved_student_id"`
	SwappedStudentID int64 `json:"swapped_student_id,omitempty"`
	TargetSeatNo    int   `json:"target_seat_no"`
}


type Camera struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	DeviceID     string `json:"device_id,omitempty"`
	Protocol     string `json:"protocol"`
	StreamURL    string `json:"stream_url,omitempty"`
	SnapshotURL  string `json:"snapshot_url,omitempty"`
	Username     string `json:"username,omitempty"`
	Password     string `json:"-"`
	HasPassword  bool   `json:"has_password"`
	AuthMode     string `json:"auth_mode"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	FPS          int    `json:"fps"`
	TimeoutMS    int    `json:"timeout_ms"`
	TLSInsecure  bool   `json:"tls_insecure"`
	IsDefault    bool   `json:"is_default"`
}
