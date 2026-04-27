package store

import (
	"database/sql"
	"fmt"
	"time"

	liradb "github.com/dylanewe/lira/db"
	"github.com/dylanewe/lira/models"
)

type GoalStore struct {
	db *sql.DB
}

func NewGoalStore(db *sql.DB) *GoalStore {
	return &GoalStore{db: db}
}

// Create inserts a new Goal, assigning it the next global ticket ID.
// g.ID, g.TicketID, and g.CreatedAt are populated on success.
func (s *GoalStore) Create(g *models.Goal) error {
	ticketID, err := liradb.NextTicketID(s.db)
	if err != nil {
		return fmt.Errorf("next ticket id: %w", err)
	}
	g.TicketID = ticketID

	var createdAt string
	err = s.db.QueryRow(`
		INSERT INTO goals (ticket_id, title, description, priority, status, color, position)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		RETURNING id, created_at
	`, g.TicketID, g.Title, g.Description, g.Priority, g.Status, g.Color, g.Position).
		Scan(&g.ID, &createdAt)
	if err != nil {
		return fmt.Errorf("insert goal: %w", err)
	}

	g.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
	return nil
}

// GetAll returns all goals ordered by status then position.
func (s *GoalStore) GetAll() ([]*models.Goal, error) {
	rows, err := s.db.Query(`
		SELECT id, ticket_id, title, description, priority, status, color, position, created_at
		FROM goals
		ORDER BY status, position
	`)
	if err != nil {
		return nil, fmt.Errorf("query goals: %w", err)
	}
	defer rows.Close()

	return scanGoals(rows)
}

// GetByID returns a single goal by its primary key.
func (s *GoalStore) GetByID(id int64) (*models.Goal, error) {
	row := s.db.QueryRow(`
		SELECT id, ticket_id, title, description, priority, status, color, position, created_at
		FROM goals WHERE id = ?
	`, id)

	g, err := scanGoal(row)
	if err != nil {
		return nil, fmt.Errorf("get goal %d: %w", id, err)
	}
	return g, nil
}

// UpdateStatus sets the status of a goal.
// If status is Done, all linked non-done steps and tasks are also set to Done.
func (s *GoalStore) UpdateStatus(id int64, status models.Status) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE goals SET status = ? WHERE id = ?`, status, id); err != nil {
		return fmt.Errorf("update goal status: %w", err)
	}

	if status == models.StatusDone {
		// Mark all linked steps done.
		if _, err := tx.Exec(`UPDATE steps SET status = ? WHERE goal_id = ?`, status, id); err != nil {
			return fmt.Errorf("cascade step status: %w", err)
		}
		// Mark all linked tasks done (directly linked or via a step of this goal).
		if _, err := tx.Exec(`
			UPDATE tasks SET status = ?
			WHERE goal_id = ?
			   OR step_id IN (SELECT id FROM steps WHERE goal_id = ?)
		`, status, id, id); err != nil {
			return fmt.Errorf("cascade task status: %w", err)
		}
	}

	return tx.Commit()
}

// UpdatePosition updates the display position of a goal within its column.
func (s *GoalStore) UpdatePosition(id int64, position int) error {
	_, err := s.db.Exec(`UPDATE goals SET position = ? WHERE id = ?`, position, id)
	return err
}

// Delete removes a goal and all linked non-Done steps and tasks.
// Steps and tasks already in Done are preserved (their goal_id is set to NULL).
func (s *GoalStore) Delete(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Collect step IDs linked to this goal so we can cascade to their tasks.
	rows, err := tx.Query(`SELECT id FROM steps WHERE goal_id = ? AND status != 'done'`, id)
	if err != nil {
		return fmt.Errorf("query non-done steps: %w", err)
	}
	var stepIDs []int64
	for rows.Next() {
		var sid int64
		if err := rows.Scan(&sid); err != nil {
			rows.Close()
			return err
		}
		stepIDs = append(stepIDs, sid)
	}
	rows.Close()

	// Delete non-done tasks belonging to those steps.
	for _, sid := range stepIDs {
		if _, err := tx.Exec(`DELETE FROM tasks WHERE step_id = ? AND status != 'done'`, sid); err != nil {
			return fmt.Errorf("delete tasks for step %d: %w", sid, err)
		}
	}

	// Delete non-done steps.
	if _, err := tx.Exec(`DELETE FROM steps WHERE goal_id = ? AND status != 'done'`, id); err != nil {
		return fmt.Errorf("delete non-done steps: %w", err)
	}

	// Nullify goal_id on done steps/tasks so they are not orphaned with an invalid FK.
	if _, err := tx.Exec(`UPDATE steps SET goal_id = NULL WHERE goal_id = ? AND status = 'done'`, id); err != nil {
		return fmt.Errorf("nullify done step goal_id: %w", err)
	}

	// Delete non-done tasks directly linked to the goal (no step).
	if _, err := tx.Exec(`DELETE FROM tasks WHERE goal_id = ? AND step_id IS NULL AND status != 'done'`, id); err != nil {
		return fmt.Errorf("delete non-done direct tasks: %w", err)
	}
	if _, err := tx.Exec(`UPDATE tasks SET goal_id = NULL WHERE goal_id = ? AND status = 'done'`, id); err != nil {
		return fmt.Errorf("nullify done task goal_id: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM goals WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete goal: %w", err)
	}

	return tx.Commit()
}

// --- helpers ---

func scanGoals(rows *sql.Rows) ([]*models.Goal, error) {
	var goals []*models.Goal
	for rows.Next() {
		g := &models.Goal{}
		var createdAt string
		err := rows.Scan(&g.ID, &g.TicketID, &g.Title, &g.Description,
			&g.Priority, &g.Status, &g.Color, &g.Position, &createdAt)
		if err != nil {
			return nil, err
		}
		g.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
		goals = append(goals, g)
	}
	return goals, rows.Err()
}

func scanGoal(row *sql.Row) (*models.Goal, error) {
	g := &models.Goal{}
	var createdAt string
	err := row.Scan(&g.ID, &g.TicketID, &g.Title, &g.Description,
		&g.Priority, &g.Status, &g.Color, &g.Position, &createdAt)
	if err != nil {
		return nil, err
	}
	g.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
	return g, nil
}
