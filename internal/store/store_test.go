package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestStudentMultipleFaceSamplesAndAttendanceFlow(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	student, err := s.CreateStudent(ctx, "2026001", "Amy", "A1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddFaceSample(ctx, student.ID, "正面", []byte{1, 2, 3, 4}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddFaceSample(ctx, student.ID, "左侧", []byte{5, 6, 7, 8}); err != nil {
		t.Fatal(err)
	}

	students, err := s.ListStudents(ctx)
	if err != nil || len(students) != 1 || !students[0].HasFace || students[0].FaceCount != 2 {
		t.Fatalf("unexpected students: %#v err=%v", students, err)
	}
	samples, err := s.ListStudentFaceSamples(ctx, student.ID)
	if err != nil || len(samples) != 2 {
		t.Fatalf("unexpected samples: %#v err=%v", samples, err)
	}

	now := time.Date(2026, 9, 21, 8, 30, 0, 0, time.Local)
	firstRecord, created, err := s.MarkAttendance(ctx, student, 0.88, now)
	if err != nil || !created {
		t.Fatalf("first attendance created=%v err=%v", created, err)
	}
	if firstRecord.CheckedAt != "2026-09-21 08:30:00" || firstRecord.LastSeenAt != firstRecord.CheckedAt || firstRecord.RecognitionCount != 1 {
		t.Fatalf("unexpected first attendance: %#v", firstRecord)
	}
	secondRecord, created, err := s.MarkAttendance(ctx, student, 0.91, now.Add(time.Hour))
	if err != nil || created {
		t.Fatalf("second attendance created=%v err=%v", created, err)
	}
	if secondRecord.CheckedAt != firstRecord.CheckedAt {
		t.Fatalf("repeat recognition changed first check-in: first=%s second=%s", firstRecord.CheckedAt, secondRecord.CheckedAt)
	}
	if secondRecord.LastSeenAt != "2026-09-21 09:30:00" || secondRecord.RecognitionCount != 2 {
		t.Fatalf("repeat recognition did not update latest/count: %#v", secondRecord)
	}
	items, err := s.ListAttendance(ctx, "2026-09-21")
	if err != nil || len(items) != 1 || items[0].CheckedAt != firstRecord.CheckedAt || items[0].LastSeenAt != secondRecord.LastSeenAt || items[0].RecognitionCount != 2 {
		t.Fatalf("unexpected attendance list: %#v err=%v", items, err)
	}
}

func TestAttendanceRecognitionColumnsMigrate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-attendance.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE students (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			student_no TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			class_name TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL
		);
		CREATE TABLE attendance (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			student_id INTEGER NOT NULL,
			day TEXT NOT NULL,
			checked_at INTEGER NOT NULL,
			similarity REAL NOT NULL,
			UNIQUE(student_id, day)
		);
		INSERT INTO students(id,student_no,name,class_name,created_at)
		VALUES(1,'LEGACY-A1','Legacy Attendance','A1',1);
		INSERT INTO attendance(id,student_id,day,checked_at,similarity)
		VALUES(1,1,'2026-09-21',1000,0.88);
	`)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	var checkedAt, lastSeenAt int64
	var recognitionCount int
	if err := store.db.QueryRow("SELECT checked_at,last_seen_at,recognition_count FROM attendance WHERE id=1").
		Scan(&checkedAt, &lastSeenAt, &recognitionCount); err != nil {
		t.Fatal(err)
	}
	if lastSeenAt != checkedAt || recognitionCount != 1 {
		t.Fatalf("legacy attendance was not backfilled: checked=%d latest=%d count=%d", checkedAt, lastSeenAt, recognitionCount)
	}
}

func TestOldSingleFaceSchemaMigratesToMultipleSamples(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE students (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			student_no TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			class_name TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL
		);
		CREATE TABLE face_samples (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			student_id INTEGER NOT NULL UNIQUE,
			embedding BLOB NOT NULL,
			created_at INTEGER NOT NULL,
			FOREIGN KEY(student_id) REFERENCES students(id) ON DELETE CASCADE
		);
		INSERT INTO students(id,student_no,name,class_name,created_at) VALUES(1,'LEGACY001','Legacy','A1',1);
		INSERT INTO face_samples(id,student_id,embedding,created_at) VALUES(1,1,x'01020304',1);
	`)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	if _, err := s.AddFaceSample(ctx, 1, "右侧", []byte{5, 6, 7, 8}); err != nil {
		t.Fatalf("adding second sample after migration: %v", err)
	}
	students, err := s.ListStudents(ctx)
	if err != nil || len(students) != 1 || students[0].FaceCount != 2 {
		t.Fatalf("legacy migration did not preserve/add samples: %#v err=%v", students, err)
	}
}


func TestClassManagementAndStudentLinkage(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "classes.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	first, err := s.CreateClass(ctx, "高一1班")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.CreateClass(ctx, "高一2班")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateClass(ctx, "高一1班"); err == nil {
		t.Fatal("expected duplicate class name to fail")
	}

	if err := s.MoveClass(ctx, second.ID, "up"); err != nil {
		t.Fatal(err)
	}
	classes, err := s.ListClasses(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(classes) != 2 || classes[0].ID != second.ID || classes[1].ID != first.ID {
		t.Fatalf("unexpected class order: %#v", classes)
	}

	student, err := s.CreateStudent(ctx, "2026002", "Bob", "高一1班")
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := s.RenameClass(ctx, first.ID, "高一3班")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != "高一3班" || renamed.StudentCount != 1 {
		t.Fatalf("unexpected renamed class: %#v", renamed)
	}

	students, err := s.ListStudents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(students) != 1 || students[0].ID != student.ID || students[0].ClassName != "高一3班" {
		t.Fatalf("class rename did not update student: %#v", students)
	}
	if err := s.DeleteClass(ctx, first.ID); err == nil {
		t.Fatal("expected class with students to be protected from deletion")
	}
	if err := s.DeleteClass(ctx, second.ID); err != nil {
		t.Fatalf("delete unused class: %v", err)
	}
}

func TestExistingStudentClassesAreImported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-classes.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE students (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			student_no TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			class_name TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL
		);
		CREATE TABLE face_samples (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			student_id INTEGER NOT NULL,
			embedding BLOB NOT NULL,
			label TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			FOREIGN KEY(student_id) REFERENCES students(id) ON DELETE CASCADE
		);
		CREATE TABLE attendance (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			student_id INTEGER NOT NULL,
			day TEXT NOT NULL,
			checked_at INTEGER NOT NULL,
			similarity REAL NOT NULL,
			FOREIGN KEY(student_id) REFERENCES students(id) ON DELETE CASCADE,
			UNIQUE(student_id, day)
		);
		INSERT INTO students(student_no,name,class_name,created_at) VALUES
			('A01','甲','高二1班',1),
			('A02','乙','高二2班',1),
			('A03','丙','高二1班',1);
	`)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	classes, err := s.ListClasses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(classes) != 2 || classes[0].Name != "高二1班" || classes[0].StudentCount != 2 || classes[1].Name != "高二2班" {
		t.Fatalf("unexpected imported classes: %#v", classes)
	}
}


func TestUpdateStudentProfileAndDuplicateStudentNo(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "update-profile.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	classA, err := s.CreateClassWithLayout(ctx, "原班级", 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateClassWithLayout(ctx, "新班级", 2, 3); err != nil {
		t.Fatal(err)
	}
	first, err := s.CreateStudent(ctx, "P001", "原姓名", classA.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateStudentSeat(ctx, first.ID, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateStudent(ctx, "P002", "其他学生", classA.Name); err != nil {
		t.Fatal(err)
	}

	updated, err := s.UpdateStudentProfile(ctx, first.ID, "P009", "新姓名", "新班级")
	if err != nil {
		t.Fatal(err)
	}
	if updated.StudentNo != "P009" || updated.Name != "新姓名" || updated.ClassName != "新班级" || updated.SeatNo != 0 {
		t.Fatalf("unexpected updated student: %#v", updated)
	}

	if _, err := s.UpdateStudentProfile(ctx, first.ID, "P002", "新姓名", "新班级"); err == nil {
		t.Fatal("expected duplicate student number update to fail")
	}
}

func TestUpdateStudentClass(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "update-class.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	if _, err := s.CreateClass(ctx, "高三1班"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateClass(ctx, "高三2班"); err != nil {
		t.Fatal(err)
	}
	student, err := s.CreateStudent(ctx, "C001", "测试学生", "高三1班")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateStudentClass(ctx, student.ID, "高三2班"); err != nil {
		t.Fatal(err)
	}
	students, err := s.ListStudents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(students) != 1 || students[0].ClassName != "高三2班" {
		t.Fatalf("student class not updated: %#v", students)
	}
}


func TestSeatLayoutAndAttendanceBoard(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "seats.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	class, err := s.CreateClassWithSettings(ctx, "高三7班", 2, 3, "08:05", "12:00")
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.CreateStudent(ctx, "S001", "甲", class.Name)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.CreateStudent(ctx, "S002", "乙", class.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateStudentSeat(ctx, first.ID, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateStudentSeat(ctx, second.ID, 2); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateStudentSeat(ctx, second.ID, 1); err == nil {
		t.Fatal("expected duplicate seat number to fail")
	}
	if _, _, err := s.MarkAttendance(ctx, first, 0.91, time.Date(2026, 9, 21, 8, 0, 0, 0, time.Local)); err != nil {
		t.Fatal(err)
	}
	if _, created, err := s.MarkAttendance(ctx, first, 0.93, time.Date(2026, 9, 21, 9, 0, 0, 0, time.Local)); err != nil || created {
		t.Fatalf("repeat recognition created=%v err=%v", created, err)
	}
	board, err := s.AttendanceSeatBoardAt(ctx, class.Name, "2026-09-21", time.Date(2026, 9, 21, 10, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if board.Total != 2 || board.Signed != 1 || board.Unsigned != 1 || board.Class.SeatRows != 2 || board.Class.SeatsPerRow != 3 {
		t.Fatalf("unexpected board: %#v", board)
	}
	if len(board.Students) != 2 || board.Students[0].SeatNo != 1 || !board.Students[0].Signed || board.Students[1].SeatNo != 2 || board.Students[1].Signed {
		t.Fatalf("unexpected seat states: %#v", board.Students)
	}
	if board.Students[0].Status != "signed" || board.Students[0].CheckedAt != "08:00:00" ||
		board.Students[0].LastSeenAt != "09:00:00" || board.Students[0].RecognitionCount != 2 {
		t.Fatalf("first check-in must drive status while latest recognition updates separately: %#v", board.Students[0])
	}
	if _, err := s.UpdateClassLayout(ctx, class.ID, 1, 1); err == nil {
		t.Fatal("layout smaller than student count should fail")
	}
	if _, err := s.UpdateClassLayout(ctx, class.ID, 1, 3); err != nil {
		t.Fatal(err)
	}
	arranged, err := s.AutoArrangeSeats(ctx, class.ID)
	if err != nil || arranged != 2 {
		t.Fatalf("auto arrange count=%d err=%v", arranged, err)
	}
}


func TestMoveStudentSeatInClassToEmptySwapAndClear(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "seat-move.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	class, err := s.CreateClassWithLayout(ctx, "移动测试班", 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.CreateStudent(ctx, "M001", "甲", class.Name)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.CreateStudent(ctx, "M002", "乙", class.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateStudentSeat(ctx, first.ID, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateStudentSeat(ctx, second.ID, 2); err != nil {
		t.Fatal(err)
	}

	moved, err := s.MoveStudentSeatInClass(ctx, class.ID, first.ID, 4)
	if err != nil {
		t.Fatal(err)
	}
	if moved.MovedStudentID != first.ID || moved.SwappedStudentID != 0 || moved.TargetSeatNo != 4 {
		t.Fatalf("unexpected empty-seat move result: %#v", moved)
	}

	swapped, err := s.MoveStudentSeatInClass(ctx, class.ID, first.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if swapped.SwappedStudentID != second.ID {
		t.Fatalf("expected second student to be swapped: %#v", swapped)
	}
	students, err := s.ListStudents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var firstSeat, secondSeat int
	for _, student := range students {
		switch student.ID {
		case first.ID:
			firstSeat = student.SeatNo
		case second.ID:
			secondSeat = student.SeatNo
		}
	}
	if firstSeat != 2 || secondSeat != 4 {
		t.Fatalf("unexpected swapped seats: first=%d second=%d", firstSeat, secondSeat)
	}

	if _, err := s.MoveStudentSeatInClass(ctx, class.ID, first.ID, 0); err != nil {
		t.Fatal(err)
	}
	students, err = s.ListStudents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, student := range students {
		if student.ID == first.ID && student.SeatNo != 0 {
			t.Fatalf("cleared student still has seat %d", student.SeatNo)
		}
	}
}


func TestClassAttendanceColumnsMigrate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-seat-class.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE students (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			student_no TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			class_name TEXT NOT NULL DEFAULT '',
			seat_no INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL
		);
		CREATE TABLE classes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			sort_order INTEGER NOT NULL,
			seat_rows INTEGER NOT NULL DEFAULT 6,
			seats_per_row INTEGER NOT NULL DEFAULT 8,
			created_at INTEGER NOT NULL
		);
		CREATE TABLE face_samples (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			student_id INTEGER NOT NULL,
			embedding BLOB NOT NULL,
			label TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			FOREIGN KEY(student_id) REFERENCES students(id) ON DELETE CASCADE
		);
		CREATE TABLE attendance (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			student_id INTEGER NOT NULL,
			day TEXT NOT NULL,
			checked_at INTEGER NOT NULL,
			similarity REAL NOT NULL,
			FOREIGN KEY(student_id) REFERENCES students(id) ON DELETE CASCADE,
			UNIQUE(student_id, day)
		);
		INSERT INTO classes(name,sort_order,seat_rows,seats_per_row,created_at)
		VALUES('旧班级',1,5,8,1);
	`)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	classes, err := s.ListClasses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(classes) != 1 || classes[0].Name != "旧班级" || classes[0].LateAfter != "" || classes[0].Deadline != "" {
		t.Fatalf("legacy class time columns were not migrated safely: %#v", classes)
	}
}


func TestCameraManagementAndDefaultSelection(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "cameras.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	local, err := s.CreateCamera(ctx, CameraInput{
		Name: "教室USB",
		Kind: "local",
		DeviceID: "device-1",
		Width: 1280,
		Height: 720,
		FPS: 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !local.IsDefault || local.Protocol != "browser" {
		t.Fatalf("first camera should become default: %#v", local)
	}

	network, err := s.CreateCamera(ctx, CameraInput{
		Name: "前门网络摄像头",
		Kind: "network",
		Protocol: "rtsp",
		StreamURL: "rtsp://192.168.1.64/Streaming/Channels/101",
		SnapshotURL: "http://192.168.1.64/ISAPI/Streaming/channels/101/picture",
		Username: "admin",
		Password: "secret",
		AuthMode: "digest",
		Width: 1920,
		Height: 1080,
		FPS: 5,
		TimeoutMS: 3500,
		IsDefault: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !network.IsDefault || !network.HasPassword || network.Password != "secret" {
		t.Fatalf("network camera settings not persisted: %#v", network)
	}
	refreshedLocal, err := s.CameraByID(ctx, local.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshedLocal.IsDefault {
		t.Fatal("old camera remained default")
	}

	if _, err := s.SetDefaultCamera(ctx, local.ID); err != nil {
		t.Fatal(err)
	}
	defaultCamera, err := s.DefaultCamera(ctx)
	if err != nil || defaultCamera.ID != local.ID {
		t.Fatalf("unexpected default camera: %#v err=%v", defaultCamera, err)
	}
	if err := s.DeleteCamera(ctx, local.ID); err != nil {
		t.Fatal(err)
	}
	defaultCamera, err = s.DefaultCamera(ctx)
	if err != nil || defaultCamera.ID != network.ID {
		t.Fatalf("default camera was not promoted after delete: %#v err=%v", defaultCamera, err)
	}
}


func TestCameraAgentConfiguration(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "camera-agent.db"))
	if err != nil { t.Fatal(err) }
	defer s.Close()
	ctx := context.Background()
	hash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	camera, err := s.CreateCamera(ctx, CameraInput{
		Name: "301教室代理", Kind: "agent", AgentID: "classroom-301",
		AgentSecretHash: hash, Width: 1280, Height: 720, FPS: 2,
	})
	if err != nil { t.Fatal(err) }
	if camera.Protocol != "agent" || camera.AgentID != "classroom-301" || !camera.HasAgentSecret || camera.AgentSecretHash != hash {
		t.Fatalf("unexpected agent camera: %#v", camera)
	}
	if _, err := s.CameraByAgentID(ctx, "classroom-301"); err != nil { t.Fatal(err) }
	if _, err := s.CreateCamera(ctx, CameraInput{
		Name: "重复Agent", Kind: "agent", AgentID: "classroom-301",
		AgentSecretHash: hash, Width: 1280, Height: 720, FPS: 2,
	}); err == nil {
		t.Fatal("expected duplicate agent id to fail")
	}
}
