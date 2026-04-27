package models

// Priority represents the urgency level of any ticket type.
type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityMedium Priority = "medium"
	PriorityHigh   Priority = "high"
)

// Status represents which kanban column a ticket sits in.
type Status string

const (
	StatusTodo       Status = "todo"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
)

// SprintStatus tracks whether a sprint is ongoing or has been closed.
type SprintStatus string

const (
	SprintActive SprintStatus = "active"
	SprintClosed SprintStatus = "closed"
)
