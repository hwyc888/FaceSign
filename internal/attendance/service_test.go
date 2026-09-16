package attendance

import (
	"testing"
	"time"

	"github.com/hwyc888/FaceSign/internal/domain"
)

func TestDetermineStatus(t *testing.T) {
	start := time.Date(2026, 9, 16, 8, 0, 0, 0, time.Local)
	cases := []struct {
		name string
		at   time.Time
		want string
	}{
		{"early", start.Add(-10 * time.Minute), domain.AttendanceOnTime},
		{"at start", start, domain.AttendanceOnTime},
		{"within grace", start.Add(5 * time.Minute), domain.AttendanceOnTime},
		{"late", start.Add(5*time.Minute + time.Second), domain.AttendanceLate},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DetermineStatus(tc.at, start, 5); got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}
