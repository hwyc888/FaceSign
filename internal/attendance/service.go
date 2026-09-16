package attendance

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/hwyc888/FaceSign/internal/domain"
	"github.com/hwyc888/FaceSign/internal/face"
	"github.com/hwyc888/FaceSign/internal/realtime"
	"github.com/hwyc888/FaceSign/internal/repository"
)

type Service struct {
	repo     *repository.Repository
	faces    face.Provider
	hub      *realtime.Hub
	location *time.Location
	logger   *slog.Logger
}

type RecognitionResult struct {
	Student domain.Student           `json:"student"`
	Session domain.AttendanceSession `json:"session"`
	Record  domain.AttendanceRecord  `json:"record"`
}

func New(repo *repository.Repository, faces face.Provider, hub *realtime.Hub, location *time.Location, logger *slog.Logger) *Service {
	return &Service{repo: repo, faces: faces, hub: hub, location: location, logger: logger}
}

func (s *Service) FaceProvider() face.Provider { return s.faces }

func (s *Service) EnrollFace(ctx context.Context, studentID int64, image []byte) (string, error) {
	student, err := s.repo.StudentByID(ctx, studentID)
	if err != nil {
		return "", err
	}
	if !student.Active {
		return "", fmt.Errorf("student is inactive")
	}
	return s.faces.Enroll(ctx, student.FaceSubject, image)
}

func (s *Service) Recognize(ctx context.Context, classroom string, image []byte, now time.Time) (RecognitionResult, error) {
	classroom = strings.TrimSpace(classroom)
	if classroom == "" {
		return RecognitionResult{}, fmt.Errorf("classroom is required")
	}
	match, err := s.faces.Recognize(ctx, image)
	if err != nil {
		return RecognitionResult{}, err
	}
	student, err := s.repo.StudentByFaceSubject(ctx, match.Subject)
	if err != nil {
		return RecognitionResult{}, fmt.Errorf("recognized face is not linked to an active student: %w", err)
	}
	if !student.Active {
		return RecognitionResult{}, fmt.Errorf("recognized student is inactive")
	}

	localNow := now.In(s.location)
	schedule, err := s.repo.FindActiveSchedule(ctx, classroom, weekdayNumber(localNow), localNow.Hour()*60+localNow.Minute())
	if err != nil {
		return RecognitionResult{}, err
	}

	enrolled, err := s.repo.StudentEnrolled(ctx, schedule.CourseID, student.ID)
	if err != nil {
		return RecognitionResult{}, err
	}
	if !enrolled {
		return RecognitionResult{}, fmt.Errorf("student %s is not enrolled in the current course", student.Name)
	}

	session, _, err := s.ensureSession(ctx, schedule, localNow)
	if err != nil {
		return RecognitionResult{}, err
	}
	if now.Unix() >= session.EndAt {
		return RecognitionResult{}, fmt.Errorf("attendance window has closed")
	}

	status := DetermineStatus(now, time.Unix(session.StartAt, 0), schedule.GraceMinutes)
	record, err := s.repo.RecordRecognition(ctx, session.ID, student.ID, status, now, match.Similarity)
	if err != nil {
		return RecognitionResult{}, err
	}

	result := RecognitionResult{Student: student, Session: session, Record: record}
	s.hub.Publish(realtime.Event{Type: "attendance.updated", Data: result})
	return result, nil
}

func DetermineStatus(at, startsAt time.Time, graceMinutes int) string {
	deadline := startsAt.Add(time.Duration(graceMinutes) * time.Minute)
	if !at.After(deadline) {
		return domain.AttendanceOnTime
	}
	return domain.AttendanceLate
}

func (s *Service) Tick(ctx context.Context, now time.Time) error {
	localNow := now.In(s.location)
	weekday := weekdayNumber(localNow)
	minute := localNow.Hour()*60 + localNow.Minute()
	schedules, err := s.repo.SchedulesForWeekday(ctx, weekday)
	if err != nil {
		return err
	}

	for _, schedule := range schedules {
		if minute < schedule.StartMinute-schedule.CheckinBeforeMinutes {
			continue
		}
		session, _, err := s.ensureSession(ctx, schedule, localNow)
		if err != nil {
			s.logger.Error("ensure attendance session", "schedule_id", schedule.ID, "error", err)
			continue
		}
		if err := s.repo.ApplyLeavesToSession(ctx, session); err != nil {
			s.logger.Error("apply leave records", "session_id", session.ID, "error", err)
		}
	}

	due, err := s.repo.SessionsDueForFinalization(ctx, now)
	if err != nil {
		return err
	}
	for _, session := range due {
		if err := s.repo.ApplyLeavesToSession(ctx, session); err != nil {
			s.logger.Error("apply leave before finalize", "session_id", session.ID, "error", err)
			continue
		}
		if err := s.repo.FinalizeSession(ctx, session); err != nil {
			s.logger.Error("finalize attendance session", "session_id", session.ID, "error", err)
			continue
		}
		s.hub.Publish(realtime.Event{Type: "attendance.session.closed", Data: map[string]any{"session_id": session.ID}})
	}
	return nil
}

func (s *Service) ensureSession(ctx context.Context, schedule domain.Schedule, localNow time.Time) (domain.AttendanceSession, bool, error) {
	date := localNow.Format("2006-01-02")
	midnight := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, s.location)
	startAt := midnight.Add(time.Duration(schedule.StartMinute) * time.Minute)
	endAt := midnight.Add(time.Duration(schedule.EndMinute) * time.Minute)
	return s.repo.EnsureAttendanceSession(ctx, schedule, date, startAt, endAt)
}

func (s *Service) EnsureCurrentSessions(ctx context.Context, now time.Time) error {
	return s.Tick(ctx, now)
}

func (s *Service) Dashboard(ctx context.Context, user domain.User, now time.Time) ([]domain.SessionSummary, error) {
	if err := s.Tick(ctx, now); err != nil && !errors.Is(err, context.Canceled) {
		s.logger.Warn("attendance tick before dashboard failed", "error", err)
	}
	teacherID := int64(0)
	if user.Role == domain.RoleTeacher {
		teacherID = user.ID
	}
	return s.repo.DashboardSummaries(ctx, now.In(s.location).Format("2006-01-02"), teacherID, now)
}

func (s *Service) StartScheduler(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	_ = s.Tick(ctx, time.Now())
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if err := s.Tick(ctx, now); err != nil {
				s.logger.Error("attendance scheduler tick failed", "error", err)
			}
		}
	}
}

func weekdayNumber(t time.Time) int {
	if t.Weekday() == time.Sunday {
		return 7
	}
	return int(t.Weekday())
}
