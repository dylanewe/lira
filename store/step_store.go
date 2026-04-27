package store

import (
	"database/sql"
	"fmt"
	"time"

	liradb "github.com/dylanewe/lira/db"
	"github.com/dylanewe/lira/models"
)

type StepStore struct {
	db *sql.DB
}

func NewStepStore(db *sql.DB) *StepStore {
	return &StepStore{db: db}
}

// Create inserts a new Step, assigning it the next global ticket ID.
// s.ID, s.TicketID, and s.CreatedAt are populated on success.
func (s *StepStore) Create(step *models.Step) error {
	ticketID, err := liradb.NextTicketID(s.db)
	if err != nil {
		return fmt.Errorf("next ticket id: %w", err)
	}
	step.TicketID = ticketID

	var createdAt string
	err = s.db.QueryRow(`
		INSERT INTO steps (ticket_id, title, description, priority, status, goal_id, sprint_id, position)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id, created_at
	`, step.TicketID, step.Title, step.Description, step.Priority, step.Status,
		step.GoalID, step.SprintID, step.Position).
		Scan(&step.ID, &createdAt)
	if err != nil {
		return fmt.Errorf("insert step: %w", err)
	}

	step.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
	return nil
}

// GetBySprintID returns all steps for a sprint, with their parent Goal hydrated
// where available. Uses LEFT JOIN so orphaned steps (goal deleted) are included.
// Results are ordered by status then position (for kanban rendering).
func (s *StepStore) GetBySprintID(sprintID int64) ([]*models.Step, error) {
	rows, err := s.db.Query(`
		SELECT
			st.id, st.ticket_id, st.title, st.description, st.priority,
			st.status, st.goal_id, st.sprint_id, st.position, st.created_at,
			g.id, g.ticket_id, g.title, g.description, g.priority,
			g.status, g.color, g.position, g.created_at
		FROM steps st
		LEFT JOIN goals g ON g.id = st.goal_id
		WHERE st.sprint_id = ?
		ORDER BY st.status, st.position
	`, sprintID)
	if err != nil {
		return nil, fmt.Errorf("query steps for sprint %d: %w", sprintID, err)
	}
	defer rows.Close()

	return scanStepsWithGoal(rows)
}

// GetByGoalID returns all steps linked to a goal (used during goal deletion cascade).
func (s *StepStore) GetByGoalID(goalID int64) ([]*models.Step, error) {
	rows, err := s.db.Query(`
		SELECT id, ticket_id, title, description, priority,
		       status, goal_id, sprint_id, position, created_at
		FROM steps WHERE goal_id = ?
	`, goalID)
	if err != nil {
		return nil, fmt.Errorf("query steps for goal %d: %w", goalID, err)
	}
	defer rows.Close()

	return scanSteps(rows)
}

// GetByID returns a single step by its primary key, with its parent Goal hydrated.
func (s *StepStore) GetByID(id int64) (*models.Step, error) {
	row := s.db.QueryRow(`
		SELECT
			st.id, st.ticket_id, st.title, st.description, st.priority,
			st.status, st.goal_id, st.sprint_id, st.position, st.created_at,
			g.id, g.ticket_id, g.title, g.description, g.priority,
			g.status, g.color, g.position, g.created_at
		FROM steps st
		JOIN goals g ON g.id = st.goal_id
		WHERE st.id = ?
	`, id)

	step, err := scanStepWithGoal(row)
	if err != nil {
		return nil, fmt.Errorf("get step %d: %w", id, err)
	}
	return step, nil
}

// UpdateStatus sets the status of a step.
func (s *StepStore) UpdateStatus(id int64, status models.Status) error {
	_, err := s.db.Exec(`UPDATE steps SET status = ? WHERE id = ?`, status, id)
	return err
}

// UpdatePosition updates the display position of a step within its column.
func (s *StepStore) UpdatePosition(id int64, position int) error {
	_, err := s.db.Exec(`UPDATE steps SET position = ? WHERE id = ?`, position, id)
	return err
}

// UpdateSprintID moves a step to a new sprint (used during carry-over).
func (s *StepStore) UpdateSprintID(id int64, sprintID int64) error {
	_, err := s.db.Exec(`UPDATE steps SET sprint_id = ? WHERE id = ?`, sprintID, id)
	return err
}

// Delete removes a step and all linked non-Done tasks.
// Done tasks have their step_id nullified so they are preserved.
func (s *StepStore) Delete(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM tasks WHERE step_id = ? AND status != 'done'`, id); err != nil {
		return fmt.Errorf("delete non-done tasks: %w", err)
	}
	if _, err := tx.Exec(`UPDATE tasks SET step_id = NULL WHERE step_id = ? AND status = 'done'`, id); err != nil {
		return fmt.Errorf("nullify done task step_id: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM steps WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete step: %w", err)
	}

	return tx.Commit()
}

// --- helpers ---

func scanStepsWithGoal(rows *sql.Rows) ([]*models.Step, error) {
	var steps []*models.Step
	for rows.Next() {
		step := &models.Step{}
		var sCreatedAt string
		// Goal columns are nullable (LEFT JOIN; goal may have been deleted).
		var gID sql.NullInt64
		var gTicketID, gTitle, gDesc, gPriority, gStatus, gColor, gCreatedAt sql.NullString
		var gPosition sql.NullInt64

		var rawGoalID sql.NullInt64
		err := rows.Scan(
			&step.ID, &step.TicketID, &step.Title, &step.Description, &step.Priority,
			&step.Status, &rawGoalID, &step.SprintID, &step.Position, &sCreatedAt,
			&gID, &gTicketID, &gTitle, &gDesc, &gPriority,
			&gStatus, &gColor, &gPosition, &gCreatedAt,
		)
		if err != nil {
			return nil, err
		}
		step.GoalID = rawGoalID.Int64
		step.CreatedAt, _ = time.Parse(time.DateTime, sCreatedAt)
		if gID.Valid {
			goal := &models.Goal{
				ID:       gID.Int64,
				TicketID: gTicketID.String,
				Title:    gTitle.String,
				Color:    gColor.String,
				Position: int(gPosition.Int64),
			}
			goal.Priority = models.Priority(gPriority.String)
			goal.Status = models.Status(gStatus.String)
			goal.CreatedAt, _ = time.Parse(time.DateTime, gCreatedAt.String)
			step.Goal = goal
		}
		steps = append(steps, step)
	}
	return steps, rows.Err()
}

func scanStepWithGoal(row *sql.Row) (*models.Step, error) {
	step := &models.Step{}
	var sCreatedAt string
	var gID sql.NullInt64
	var gTicketID, gTitle, gDesc, gPriority, gStatus, gColor, gCreatedAt sql.NullString
	var gPosition sql.NullInt64

	var rawGoalID sql.NullInt64
	err := row.Scan(
		&step.ID, &step.TicketID, &step.Title, &step.Description, &step.Priority,
		&step.Status, &rawGoalID, &step.SprintID, &step.Position, &sCreatedAt,
		&gID, &gTicketID, &gTitle, &gDesc, &gPriority,
		&gStatus, &gColor, &gPosition, &gCreatedAt,
	)
	if err != nil {
		return nil, err
	}
	step.GoalID = rawGoalID.Int64
	step.CreatedAt, _ = time.Parse(time.DateTime, sCreatedAt)
	if gID.Valid {
		goal := &models.Goal{
			ID:       gID.Int64,
			TicketID: gTicketID.String,
			Title:    gTitle.String,
			Color:    gColor.String,
			Position: int(gPosition.Int64),
		}
		goal.Priority = models.Priority(gPriority.String)
		goal.Status = models.Status(gStatus.String)
		goal.CreatedAt, _ = time.Parse(time.DateTime, gCreatedAt.String)
		step.Goal = goal
	}
	return step, nil
}

func scanSteps(rows *sql.Rows) ([]*models.Step, error) {
	var steps []*models.Step
	for rows.Next() {
		step := &models.Step{}
		var createdAt string
		var rawGoalID sql.NullInt64
		err := rows.Scan(
			&step.ID, &step.TicketID, &step.Title, &step.Description, &step.Priority,
			&step.Status, &rawGoalID, &step.SprintID, &step.Position, &createdAt,
		)
		if err != nil {
			return nil, err
		}
		step.GoalID = rawGoalID.Int64
		step.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
		steps = append(steps, step)
	}
	return steps, rows.Err()
}
