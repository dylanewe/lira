package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/dylanewe/lira/config"
	"github.com/dylanewe/lira/models"
)

// itemKind distinguishes steps from tasks on the kanban board.
type itemKind int

const (
	kindStep itemKind = iota
	kindTask
)

// boardItem is a flat, renderable representation of a Step or Task.
// Child tasks (linked to a step) carry isChild=true for the ↳ prefix.
type boardItem struct {
	kind    itemKind
	isChild bool // task linked to a step in the same column
	step    *models.Step
	task    *models.Task
}

func (b boardItem) id() string {
	if b.kind == kindStep {
		return b.step.TicketID
	}
	return b.task.TicketID
}

func (b boardItem) title() string {
	if b.kind == kindStep {
		return b.step.Title
	}
	return b.task.Title
}

func (b boardItem) priority() models.Priority {
	if b.kind == kindStep {
		return b.step.Priority
	}
	return b.task.Priority
}

func (b boardItem) colorName() string {
	if b.kind == kindStep && b.step.Goal != nil {
		return b.step.Goal.Color
	}
	if b.kind == kindTask && b.task.Goal != nil {
		return b.task.Goal.Color
	}
	return ""
}

func (b boardItem) status() models.Status {
	if b.kind == kindStep {
		return b.step.Status
	}
	return b.task.Status
}

func (b boardItem) repeatable() bool {
	return b.kind == kindTask && b.task.Repeatable
}

func (b boardItem) dbID() int64 {
	if b.kind == kindStep {
		return b.step.ID
	}
	return b.task.ID
}

// boardCol holds the flat item list for one kanban column.
type boardCol struct {
	status models.Status
	items  []boardItem
}

// buildCols organises loaded steps and tasks into the three kanban columns.
// Within each column, steps come first followed by their same-status child
// tasks, then cross-status child tasks, then standalone tasks.
func buildCols(steps []*models.Step, tasks []*models.Task) [3]boardCol {
	cols := [3]boardCol{
		{status: models.StatusTodo},
		{status: models.StatusInProgress},
		{status: models.StatusDone},
	}

	colIdx := func(s models.Status) int {
		switch s {
		case models.StatusInProgress:
			return 1
		case models.StatusDone:
			return 2
		default:
			return 0
		}
	}

	stepByID := make(map[int64]*models.Step, len(steps))
	for _, s := range steps {
		stepByID[s.ID] = s
	}

	// Group child tasks by their parent step ID.
	childByStepID := make(map[int64][]*models.Task)
	var standaloneTasks []*models.Task
	for _, t := range tasks {
		if t.StepID.Valid {
			childByStepID[t.StepID.Int64] = append(childByStepID[t.StepID.Int64], t)
		} else {
			standaloneTasks = append(standaloneTasks, t)
		}
	}

	// Add steps + same-status children.
	for _, s := range steps {
		ci := colIdx(s.Status)
		cols[ci].items = append(cols[ci].items, boardItem{kind: kindStep, step: s})
		for _, t := range childByStepID[s.ID] {
			if t.Status == s.Status {
				cols[ci].items = append(cols[ci].items, boardItem{
					kind: kindTask, isChild: true, task: t,
				})
			}
		}
	}

	// Add child tasks whose parent step is in a different column.
	for _, t := range tasks {
		if !t.StepID.Valid {
			continue
		}
		parent, ok := stepByID[t.StepID.Int64]
		if !ok || parent.Status == t.Status {
			continue // already handled or parent missing
		}
		ci := colIdx(t.Status)
		cols[ci].items = append(cols[ci].items, boardItem{
			kind: kindTask, isChild: true, task: t,
		})
	}

	// Add standalone tasks.
	for _, t := range standaloneTasks {
		ci := colIdx(t.Status)
		cols[ci].items = append(cols[ci].items, boardItem{kind: kindTask, task: t})
	}

	return cols
}

// --- rendering helpers ---

// goalHex returns the lipgloss color for a named goal color.
// Falls back to the foreground color for standalone (unlinked) items.
func goalHex(name string) lipgloss.Color {
	if c, ok := config.GoalColorByName(name); ok {
		return lipgloss.Color(c.Hex)
	}
	return lipgloss.Color(config.ColorSubtle)
}

// priorityBadge returns a short styled string indicating priority level.
func priorityBadge(p models.Priority) string {
	switch p {
	case models.PriorityHigh:
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(config.ColorPriorityHigh)).
			Render("!!")
	case models.PriorityMedium:
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(config.ColorPriorityMedium)).
			Render("! ")
	default:
		return "  "
	}
}

// statusColor returns the display color for a kanban column header.
func statusColor(s models.Status) lipgloss.Color {
	switch s {
	case models.StatusTodo:
		return lipgloss.Color(config.ColorTodo)
	case models.StatusInProgress:
		return lipgloss.Color(config.ColorInProgress)
	case models.StatusDone:
		return lipgloss.Color(config.ColorDone)
	default:
		return lipgloss.Color(config.ColorMuted)
	}
}

// statusLabel returns the display label for a column.
func statusLabel(s models.Status) string {
	switch s {
	case models.StatusTodo:
		return "Todo"
	case models.StatusInProgress:
		return "In Progress"
	case models.StatusDone:
		return "Done"
	default:
		return string(s)
	}
}

// truncate shortens s to maxLen visible runes, appending "…" if needed.
func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-1]) + "…"
}

// hintBar renders the keybinding hint line at the bottom of a board view.
func hintBar(hints []string) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(config.ColorMuted)).
		Render(strings.Join(hints, "  "))
}

// placeholderColumns renders three empty kanban columns (used by stub views).
func placeholderColumns(width int) string {
	colWidth := (width - 6) / 3
	colStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(config.ColorBorder)).
		Width(colWidth).
		Height(10)

	emptyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(config.ColorMuted)).
		Italic(true)

	col := func(title string, status models.Status) string {
		header := lipgloss.NewStyle().
			Foreground(statusColor(status)).
			Bold(true).
			Render(title)
		return colStyle.Render(header + "\n" + emptyStyle.Render("empty"))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top,
		col("Todo", models.StatusTodo),
		col("In Progress", models.StatusInProgress),
		col("Done", models.StatusDone),
	)
}
