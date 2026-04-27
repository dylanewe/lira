package models

import "time"

// Goal is the top-level ticket type (equivalent to an Epic in Jira).
// Goals are not sprint-scoped — they persist across sprints until marked Done.
// All Steps and Tasks linked to a Goal inherit its Color.
type Goal struct {
	ID          int64
	TicketID    string   // "L-0000"
	Title       string
	Description string
	Priority    Priority // "low" | "medium" | "high"
	Status      Status   // "todo" | "in_progress" | "done"
	Color       string   // Horizon palette key, e.g. "primary", "secondary"
	Position    int      // display order within status column on the Goals Board
	CreatedAt   time.Time
}
