package models

import (
	"database/sql"
	"time"
)

// Task is the smallest unit of work.
// Tasks can be:
//   - Standalone (no Step or Goal link)
//   - Linked to a Goal only
//   - Linked to a Step (and by extension, its Goal)
//
// Repeatable tasks are re-created fresh at the start of each new sprint,
// even if they were completed in the prior sprint.
// Tasks carry over to the next sprint if incomplete at sprint end.
type Task struct {
	ID          int64
	TicketID    string       // "L-0000"
	Title       string
	Description string
	Priority    Priority     // "low" | "medium" | "high"
	Status      Status       // "todo" | "in_progress" | "done"
	StepID      sql.NullInt64 // nullable; references steps.id
	GoalID      sql.NullInt64 // nullable; references goals.id
	SprintID    int64        // current sprint; updated on carry-over
	Repeatable  bool         // if true, re-created each sprint
	Position    int          // display order within status column on the Dashboard
	CreatedAt   time.Time

	// Populated by joins — not persisted directly.
	Step *Step
	Goal *Goal
}

// IsChild reports whether the task is linked to a step (shown as a child item
// in the dashboard with a ↳ prefix).
func (t Task) IsChild() bool {
	return t.StepID.Valid
}

// Color returns the display color for this task, inherited from its linked
// Goal (via Step or direct link). Returns empty string for standalone tasks.
func (t Task) Color() string {
	if t.Goal != nil {
		return t.Goal.Color
	}
	return ""
}
