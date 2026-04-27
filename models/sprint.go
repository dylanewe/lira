package models

import "time"

// Sprint represents a time-boxed work period.
// The first sprint is started manually; subsequent sprints open automatically
// on app launch when the previous sprint's end time has passed.
type Sprint struct {
	ID        int64
	Number    int          // 1, 2, 3 ...
	StartDate time.Time
	EndDate   time.Time
	Status    SprintStatus // "active" | "closed"
}

// IsExpired reports whether the sprint's end date is in the past.
func (s Sprint) IsExpired() bool {
	return time.Now().After(s.EndDate)
}

// Duration returns the configured length of the sprint.
func (s Sprint) Duration() time.Duration {
	return s.EndDate.Sub(s.StartDate)
}
