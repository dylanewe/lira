package store

import (
	"database/sql"
	"fmt"
	"time"

	liradb "github.com/dylanewe/lira/db"
	"github.com/dylanewe/lira/models"
)

type TaskStore struct {
	db *sql.DB
}

func NewTaskStore(db *sql.DB) *TaskStore {
	return &TaskStore{db: db}
}

// Create inserts a new Task, assigning it the next global ticket ID.
// t.ID, t.TicketID, and t.CreatedAt are populated on success.
func (s *TaskStore) Create(t *models.Task) error {
	ticketID, err := liradb.NextTicketID(s.db)
	if err != nil {
		return fmt.Errorf("next ticket id: %w", err)
	}
	t.TicketID = ticketID

	rep := 0
	if t.Repeatable {
		rep = 1
	}

	var createdAt string
	err = s.db.QueryRow(`
		INSERT INTO tasks (ticket_id, title, description, priority, status,
		                   step_id, goal_id, sprint_id, repeatable, position)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id, created_at
	`, t.TicketID, t.Title, t.Description, t.Priority, t.Status,
		t.StepID, t.GoalID, t.SprintID, rep, t.Position).
		Scan(&t.ID, &createdAt)
	if err != nil {
		return fmt.Errorf("insert task: %w", err)
	}

	t.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
	return nil
}

// GetBySprintID returns all tasks for a sprint, with Step and Goal hydrated
// where applicable. Results are ordered by status then position.
func (s *TaskStore) GetBySprintID(sprintID int64) ([]*models.Task, error) {
	rows, err := s.db.Query(`
		SELECT
			t.id, t.ticket_id, t.title, t.description, t.priority,
			t.status, t.step_id, t.goal_id, t.sprint_id, t.repeatable, t.position, t.created_at,
			-- step columns (nullable join)
			st.id, st.ticket_id, st.title, st.goal_id, st.sprint_id,
			-- goal columns (nullable join, via step or direct)
			g.id, g.ticket_id, g.title, g.color
		FROM tasks t
		LEFT JOIN steps st ON st.id = t.step_id
		LEFT JOIN goals g  ON g.id  = COALESCE(t.goal_id, st.goal_id)
		WHERE t.sprint_id = ?
		ORDER BY t.status, t.position
	`, sprintID)
	if err != nil {
		return nil, fmt.Errorf("query tasks for sprint %d: %w", sprintID, err)
	}
	defer rows.Close()

	return scanTasksWithJoins(rows)
}

// GetByStepID returns all tasks linked to a step (used during step deletion cascade).
func (s *TaskStore) GetByStepID(stepID int64) ([]*models.Task, error) {
	rows, err := s.db.Query(`
		SELECT id, ticket_id, title, description, priority,
		       status, step_id, goal_id, sprint_id, repeatable, position, created_at
		FROM tasks WHERE step_id = ?
	`, stepID)
	if err != nil {
		return nil, fmt.Errorf("query tasks for step %d: %w", stepID, err)
	}
	defer rows.Close()

	return scanTasksFlat(rows)
}

// GetByGoalID returns all tasks directly linked to a goal (step_id IS NULL).
func (s *TaskStore) GetByGoalID(goalID int64) ([]*models.Task, error) {
	rows, err := s.db.Query(`
		SELECT id, ticket_id, title, description, priority,
		       status, step_id, goal_id, sprint_id, repeatable, position, created_at
		FROM tasks WHERE goal_id = ? AND step_id IS NULL
	`, goalID)
	if err != nil {
		return nil, fmt.Errorf("query tasks for goal %d: %w", goalID, err)
	}
	defer rows.Close()

	return scanTasksFlat(rows)
}

// GetRepeatableBySprintID returns all repeatable tasks for a given sprint.
// Used during sprint carry-over to seed the next sprint.
func (s *TaskStore) GetRepeatableBySprintID(sprintID int64) ([]*models.Task, error) {
	rows, err := s.db.Query(`
		SELECT id, ticket_id, title, description, priority,
		       status, step_id, goal_id, sprint_id, repeatable, position, created_at
		FROM tasks WHERE sprint_id = ? AND repeatable = 1
	`, sprintID)
	if err != nil {
		return nil, fmt.Errorf("query repeatable tasks for sprint %d: %w", sprintID, err)
	}
	defer rows.Close()

	return scanTasksFlat(rows)
}

// GetByID returns a single task by its primary key, with Step and Goal hydrated.
func (s *TaskStore) GetByID(id int64) (*models.Task, error) {
	rows, err := s.db.Query(`
		SELECT
			t.id, t.ticket_id, t.title, t.description, t.priority,
			t.status, t.step_id, t.goal_id, t.sprint_id, t.repeatable, t.position, t.created_at,
			st.id, st.ticket_id, st.title, st.goal_id, st.sprint_id,
			g.id, g.ticket_id, g.title, g.color
		FROM tasks t
		LEFT JOIN steps st ON st.id = t.step_id
		LEFT JOIN goals g  ON g.id  = COALESCE(t.goal_id, st.goal_id)
		WHERE t.id = ?
		LIMIT 1
	`, id)
	if err != nil {
		return nil, fmt.Errorf("get task %d: %w", id, err)
	}
	defer rows.Close()

	tasks, err := scanTasksWithJoins(rows)
	if err != nil {
		return nil, fmt.Errorf("scan task %d: %w", id, err)
	}
	if len(tasks) == 0 {
		return nil, sql.ErrNoRows
	}
	return tasks[0], nil
}

// UpdateStatus sets the status of a task.
func (s *TaskStore) UpdateStatus(id int64, status models.Status) error {
	_, err := s.db.Exec(`UPDATE tasks SET status = ? WHERE id = ?`, status, id)
	return err
}

// UpdatePosition updates the display position of a task within its column.
func (s *TaskStore) UpdatePosition(id int64, position int) error {
	_, err := s.db.Exec(`UPDATE tasks SET position = ? WHERE id = ?`, position, id)
	return err
}

// UpdateSprintID moves a task to a new sprint (used during carry-over).
func (s *TaskStore) UpdateSprintID(id int64, sprintID int64) error {
	_, err := s.db.Exec(`UPDATE tasks SET sprint_id = ? WHERE id = ?`, sprintID, id)
	return err
}

// Delete removes a task by ID.
func (s *TaskStore) Delete(id int64) error {
	_, err := s.db.Exec(`DELETE FROM tasks WHERE id = ?`, id)
	return err
}

// --- helpers ---

// scanTasksWithJoins scans rows that include optional step and goal join columns.
func scanTasksWithJoins(rows *sql.Rows) ([]*models.Task, error) {
	var tasks []*models.Task
	for rows.Next() {
		t := &models.Task{}
		var createdAt string
		var rep int

		// Nullable step columns
		var stepID, stepGoalID, stepSprintID sql.NullInt64
		var stepTicketID, stepTitle sql.NullString

		// Nullable goal columns
		var goalID sql.NullInt64
		var goalTicketID, goalTitle, goalColor sql.NullString

		err := rows.Scan(
			&t.ID, &t.TicketID, &t.Title, &t.Description, &t.Priority,
			&t.Status, &t.StepID, &t.GoalID, &t.SprintID, &rep, &t.Position, &createdAt,
			&stepID, &stepTicketID, &stepTitle, &stepGoalID, &stepSprintID,
			&goalID, &goalTicketID, &goalTitle, &goalColor,
		)
		if err != nil {
			return nil, err
		}

		t.Repeatable = rep == 1
		t.CreatedAt, _ = time.Parse(time.DateTime, createdAt)

		if stepID.Valid {
			t.Step = &models.Step{
				ID:       stepID.Int64,
				TicketID: stepTicketID.String,
				Title:    stepTitle.String,
				GoalID:   stepGoalID.Int64,
				SprintID: stepSprintID.Int64,
			}
		}
		if goalID.Valid {
			t.Goal = &models.Goal{
				ID:       goalID.Int64,
				TicketID: goalTicketID.String,
				Title:    goalTitle.String,
				Color:    goalColor.String,
			}
			if t.Step != nil {
				t.Step.Goal = t.Goal
			}
		}

		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// scanTasksFlat scans rows with only task columns (no joins).
func scanTasksFlat(rows *sql.Rows) ([]*models.Task, error) {
	var tasks []*models.Task
	for rows.Next() {
		t := &models.Task{}
		var createdAt string
		var rep int
		err := rows.Scan(
			&t.ID, &t.TicketID, &t.Title, &t.Description, &t.Priority,
			&t.Status, &t.StepID, &t.GoalID, &t.SprintID, &rep, &t.Position, &createdAt,
		)
		if err != nil {
			return nil, err
		}
		t.Repeatable = rep == 1
		t.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

