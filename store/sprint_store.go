package store

import (
	"database/sql"
	"fmt"
	"time"

	liradb "github.com/dylanewe/lira/db"
	"github.com/dylanewe/lira/models"
)

type SprintStore struct {
	db *sql.DB
}

func NewSprintStore(db *sql.DB) *SprintStore {
	return &SprintStore{db: db}
}

// Create inserts a new sprint record.
// s.ID is populated on success.
func (s *SprintStore) Create(sprint *models.Sprint) error {
	err := s.db.QueryRow(`
		INSERT INTO sprints (number, start_date, end_date, status)
		VALUES (?, ?, ?, ?)
		RETURNING id
	`, sprint.Number,
		sprint.StartDate.UTC().Format(time.RFC3339),
		sprint.EndDate.UTC().Format(time.RFC3339),
		sprint.Status,
	).Scan(&sprint.ID)
	if err != nil {
		return fmt.Errorf("insert sprint: %w", err)
	}
	return nil
}

// GetActive returns the current active sprint, or nil if none exists yet.
func (s *SprintStore) GetActive() (*models.Sprint, error) {
	row := s.db.QueryRow(`
		SELECT id, number, start_date, end_date, status
		FROM sprints WHERE status = 'active'
		LIMIT 1
	`)
	sprint, err := scanSprint(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get active sprint: %w", err)
	}
	return sprint, nil
}

// GetByID returns a sprint by its primary key.
func (s *SprintStore) GetByID(id int64) (*models.Sprint, error) {
	row := s.db.QueryRow(`
		SELECT id, number, start_date, end_date, status
		FROM sprints WHERE id = ?
	`, id)
	sprint, err := scanSprint(row)
	if err != nil {
		return nil, fmt.Errorf("get sprint %d: %w", id, err)
	}
	return sprint, nil
}

// GetAll returns all sprints ordered by number ascending (for stats navigation).
func (s *SprintStore) GetAll() ([]*models.Sprint, error) {
	rows, err := s.db.Query(`
		SELECT id, number, start_date, end_date, status
		FROM sprints ORDER BY number ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query sprints: %w", err)
	}
	defer rows.Close()

	var sprints []*models.Sprint
	for rows.Next() {
		sprint, err := scanSprintRow(rows)
		if err != nil {
			return nil, err
		}
		sprints = append(sprints, sprint)
	}
	return sprints, rows.Err()
}

// HasAny reports whether at least one sprint exists in the database.
// Used on startup to decide whether to show the setup screen.
func (s *SprintStore) HasAny() (bool, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM sprints`).Scan(&count)
	return count > 0, err
}

// Close marks a sprint as closed.
func (s *SprintStore) Close(id int64) error {
	_, err := s.db.Exec(`UPDATE sprints SET status = 'closed' WHERE id = ?`, id)
	return err
}

// AdvanceIfExpired checks whether the active sprint has passed its end date.
// If so, it closes it and opens the next sprint, carrying over incomplete
// tickets and re-seeding repeatable tasks. Returns the current active sprint
// (either the existing one or the newly created one).
func (s *SprintStore) AdvanceIfExpired(sprintLengthDays int) (*models.Sprint, error) {
	active, err := s.GetActive()
	if err != nil {
		return nil, err
	}
	if active == nil {
		return nil, nil // no sprint started yet; setup screen handles this
	}
	if !active.IsExpired() {
		return active, nil // still within sprint window
	}

	// One or more sprints may have been missed (e.g. app not opened for weeks).
	// Advance through each expired sprint until we land on a current one.
	current := active
	for current.IsExpired() {
		next, err := s.closeAndOpenNext(current, sprintLengthDays)
		if err != nil {
			return nil, err
		}
		current = next
	}

	return current, nil
}

// closeAndOpenNext closes fromSprint, carries over its incomplete tickets, and
// opens the immediately following sprint. Returns the new active sprint.
func (s *SprintStore) closeAndOpenNext(from *models.Sprint, sprintLengthDays int) (*models.Sprint, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Close the current sprint.
	if _, err := tx.Exec(`UPDATE sprints SET status = 'closed' WHERE id = ?`, from.ID); err != nil {
		return nil, fmt.Errorf("close sprint %d: %w", from.ID, err)
	}

	// Create the next sprint immediately after the previous one ends.
	newStart := from.EndDate
	newEnd := newStart.Add(time.Duration(sprintLengthDays) * 24 * time.Hour)

	next := &models.Sprint{
		Number:    from.Number + 1,
		StartDate: newStart,
		EndDate:   newEnd,
		Status:    models.SprintActive,
	}
	err = tx.QueryRow(`
		INSERT INTO sprints (number, start_date, end_date, status)
		VALUES (?, ?, ?, ?)
		RETURNING id
	`, next.Number,
		next.StartDate.UTC().Format(time.RFC3339),
		next.EndDate.UTC().Format(time.RFC3339),
		next.Status,
	).Scan(&next.ID)
	if err != nil {
		return nil, fmt.Errorf("create sprint %d: %w", next.Number, err)
	}

	// Carry over incomplete (non-done) steps.
	if _, err := tx.Exec(`
		UPDATE steps SET sprint_id = ? WHERE sprint_id = ? AND status != 'done'
	`, next.ID, from.ID); err != nil {
		return nil, fmt.Errorf("carry over steps: %w", err)
	}

	// Carry over incomplete non-repeatable tasks.
	if _, err := tx.Exec(`
		UPDATE tasks SET sprint_id = ? WHERE sprint_id = ? AND status != 'done' AND repeatable = 0
	`, next.ID, from.ID); err != nil {
		return nil, fmt.Errorf("carry over tasks: %w", err)
	}

	// Re-seed repeatable tasks: create a fresh copy in the new sprint.
	// The original record stays in the closed sprint as a historical entry.
	repeatRows, err := tx.Query(`
		SELECT ticket_id, title, description, priority, step_id, goal_id, position
		FROM tasks WHERE sprint_id = ? AND repeatable = 1
	`, from.ID)
	if err != nil {
		return nil, fmt.Errorf("query repeatable tasks: %w", err)
	}

	type repeatSeed struct {
		title, description string
		priority           models.Priority
		stepID, goalID     sql.NullInt64
		position           int
	}
	var seeds []repeatSeed
	for repeatRows.Next() {
		var rs repeatSeed
		var oldTicketID string // discard — new ticket ID assigned below
		if err := repeatRows.Scan(&oldTicketID, &rs.title, &rs.description,
			&rs.priority, &rs.stepID, &rs.goalID, &rs.position); err != nil {
			repeatRows.Close()
			return nil, err
		}
		seeds = append(seeds, rs)
	}
	repeatRows.Close()
	if err := repeatRows.Err(); err != nil {
		return nil, err
	}

	for _, seed := range seeds {
		newTicketID, err := liradb.NextTicketID(s.db)
		if err != nil {
			return nil, fmt.Errorf("next ticket id for repeat: %w", err)
		}
		if _, err := tx.Exec(`
			INSERT INTO tasks (ticket_id, title, description, priority, status,
			                   step_id, goal_id, sprint_id, repeatable, position)
			VALUES (?, ?, ?, ?, 'todo', ?, ?, ?, 1, ?)
		`, newTicketID, seed.title, seed.description, seed.priority,
			seed.stepID, seed.goalID, next.ID, seed.position); err != nil {
			return nil, fmt.Errorf("insert repeat task: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return next, nil
}

// --- Sprint stats queries ---

// SprintStats holds computed metrics for a single sprint.
type SprintStats struct {
	Sprint          *models.Sprint
	TotalCreated    int
	TotalDone       int
	TotalCarriedIn  int // tickets that arrived via carry-over (created in a prior sprint)
	TotalCarriedOut int // incomplete tickets at sprint close
	Velocity        float64 // tickets completed per day
}

// GetStats returns computed stats for a given sprint.
func (s *SprintStore) GetStats(sprintID int64) (*SprintStats, error) {
	sprint, err := s.GetByID(sprintID)
	if err != nil {
		return nil, err
	}

	stats := &SprintStats{Sprint: sprint}

	// Count steps + tasks created in this sprint and their statuses.
	err = s.db.QueryRow(`
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN status = 'done' THEN 1 ELSE 0 END), 0)
		FROM (
			SELECT status FROM steps WHERE sprint_id = ?
			UNION ALL
			SELECT status FROM tasks WHERE sprint_id = ?
		)
	`, sprintID, sprintID).Scan(&stats.TotalCreated, &stats.TotalDone)
	if err != nil {
		return nil, fmt.Errorf("count sprint tickets: %w", err)
	}

	days := sprint.Duration().Hours() / 24
	if days > 0 {
		stats.Velocity = float64(stats.TotalDone) / days
	}

	if sprint.Status == models.SprintClosed {
		stats.TotalCarriedOut = stats.TotalCreated - stats.TotalDone
	}

	return stats, nil
}

// StreakCount returns the current streak: the number of consecutive closed
// sprints (up to and including the most recent closed sprint) in which ALL
// repeatable tasks were completed.
func (s *SprintStore) StreakCount() (int, error) {
	sprints, err := s.GetAll()
	if err != nil {
		return 0, err
	}

	streak := 0
	// Walk sprints in reverse (most recent first), stop at first broken sprint.
	for i := len(sprints) - 1; i >= 0; i-- {
		sp := sprints[i]
		if sp.Status != models.SprintClosed {
			continue
		}

		var total, done int
		err := s.db.QueryRow(`
			SELECT
				COUNT(*),
				COALESCE(SUM(CASE WHEN status = 'done' THEN 1 ELSE 0 END), 0)
			FROM tasks
			WHERE sprint_id = ? AND repeatable = 1
		`, sp.ID).Scan(&total, &done)
		if err != nil {
			return 0, fmt.Errorf("streak query sprint %d: %w", sp.ID, err)
		}

		// A sprint with no repeatable tasks does not break or contribute to the streak.
		if total == 0 {
			continue
		}
		if done < total {
			break // streak broken
		}
		streak++
	}

	return streak, nil
}

// MonthlyStats holds aggregated metrics for a calendar month.
type MonthlyStats struct {
	Year, Month     int
	TotalDone       int
	SprintVelocities []SprintVelocity // one entry per sprint that overlaps the month
	Streak          int
}

// SprintVelocity is a single sprint's velocity data point for the monthly chart.
type SprintVelocity struct {
	SprintNumber int
	Velocity     float64
}

// GetMonthlyStats returns aggregated stats for the given year and month.
func (s *SprintStore) GetMonthlyStats(year, month int) (*MonthlyStats, error) {
	ms := &MonthlyStats{Year: year, Month: month}

	// Find all sprints that overlap the requested calendar month.
	startOfMonth := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	endOfMonth := startOfMonth.AddDate(0, 1, 0)

	rows, err := s.db.Query(`
		SELECT id, number, start_date, end_date, status FROM sprints
		WHERE start_date < ? AND end_date > ?
		ORDER BY number ASC
	`, endOfMonth.Format(time.RFC3339), startOfMonth.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("query monthly sprints: %w", err)
	}
	defer rows.Close()

	var sprintIDs []int64
	var sprintMap = map[int64]*models.Sprint{}
	for rows.Next() {
		sp, err := scanSprintRow(rows)
		if err != nil {
			return nil, err
		}
		sprintIDs = append(sprintIDs, sp.ID)
		sprintMap[sp.ID] = sp
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, sid := range sprintIDs {
		sp := sprintMap[sid]
		var done int
		err := s.db.QueryRow(`
			SELECT COALESCE(SUM(CASE WHEN status = 'done' THEN 1 ELSE 0 END), 0)
			FROM (
				SELECT status FROM steps WHERE sprint_id = ?
				UNION ALL
				SELECT status FROM tasks WHERE sprint_id = ?
			)
		`, sid, sid).Scan(&done)
		if err != nil {
			return nil, err
		}

		ms.TotalDone += done

		days := sp.Duration().Hours() / 24
		vel := 0.0
		if days > 0 {
			vel = float64(done) / days
		}
		ms.SprintVelocities = append(ms.SprintVelocities, SprintVelocity{
			SprintNumber: sp.Number,
			Velocity:     vel,
		})
	}

	streak, err := s.StreakCount()
	if err != nil {
		return nil, err
	}
	ms.Streak = streak

	return ms, nil
}

// --- helpers ---

func scanSprint(row *sql.Row) (*models.Sprint, error) {
	s := &models.Sprint{}
	var start, end string
	var status string
	if err := row.Scan(&s.ID, &s.Number, &start, &end, &status); err != nil {
		return nil, err
	}
	s.StartDate, _ = time.Parse(time.RFC3339, start)
	s.EndDate, _ = time.Parse(time.RFC3339, end)
	s.Status = models.SprintStatus(status)
	return s, nil
}

func scanSprintRow(rows *sql.Rows) (*models.Sprint, error) {
	s := &models.Sprint{}
	var start, end, status string
	if err := rows.Scan(&s.ID, &s.Number, &start, &end, &status); err != nil {
		return nil, err
	}
	s.StartDate, _ = time.Parse(time.RFC3339, start)
	s.EndDate, _ = time.Parse(time.RFC3339, end)
	s.Status = models.SprintStatus(status)
	return s, nil
}
