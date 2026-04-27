package models

import "time"

// Step is a mid-level ticket (equivalent to a Story in Jira).
// Every Step must belong to a Goal and is tracked within a sprint.
// Steps carry over to the next sprint if incomplete at sprint end.
// Steps inherit their display color from their parent Goal.
type Step struct {
	ID          int64
	TicketID    string   // "L-0000"
	Title       string
	Description string
	Priority    Priority // "low" | "medium" | "high"
	Status      Status   // "todo" | "in_progress" | "done"
	GoalID      int64    // required; references goals.id
	SprintID    int64    // current sprint; updated on carry-over
	Position    int      // display order within status column on the Dashboard
	CreatedAt   time.Time

	// Populated by joins — not persisted directly.
	Goal *Goal
}
