package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/dylanewe/lira/config"
	"github.com/dylanewe/lira/models"
)

type goalsBoardMode int

const (
	goalsModeNormal   goalsBoardMode = iota
	goalsModeSelected                // item chosen, waiting for column-move key
	goalsModeConfirm                 // delete confirmation active
	goalsModeCreate                  // create-ticket form overlay
)

// GoalsBoardModel is the Goals-only kanban board (G key).
// Goals are not sprint-scoped and persist until marked Done.
// Moving a Goal to Done cascades Done status to all linked Steps and Tasks.
type GoalsBoardModel struct {
	stores Stores
	sprint *models.Sprint
	width  int
	height int

	cols   [3][]*models.Goal
	loaded bool

	col int
	row int

	mode        goalsBoardMode
	pendingGoal *models.Goal

	form *createFormModel
}

func newGoalsBoardModel(stores Stores, sprint *models.Sprint, w, h int) GoalsBoardModel {
	return GoalsBoardModel{stores: stores, sprint: sprint, width: w, height: h}
}

func (m GoalsBoardModel) Init() tea.Cmd {
	return m.loadCmd()
}

func (m GoalsBoardModel) Update(msg tea.Msg) (GoalsBoardModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.form != nil {
			m.form.width, m.form.height = msg.Width, msg.Height
		}

	case goalsLoadedMsg:
		m.cols = buildGoalCols(msg.goals)
		m.loaded = true
		m.clampCursor()

	case goalsReloadMsg:
		return m, m.loadCmd()

	case createFormDoneMsg:
		m.form = nil
		m.mode = goalsModeNormal
		if !msg.cancelled {
			return m, m.loadCmd()
		}

	case tea.KeyMsg:
		switch m.mode {
		case goalsModeCreate:
			if m.form != nil {
				updated, cmd := m.form.Update(msg)
				m.form = &updated
				return m, cmd
			}
		case goalsModeConfirm:
			return m.handleConfirmKey(msg)
		case goalsModeSelected:
			return m.handleSelectedKey(msg)
		default:
			return m.handleNormalKey(msg)
		}
	default:
		if m.mode == goalsModeCreate && m.form != nil {
			updated, cmd := m.form.Update(msg)
			m.form = &updated
			return m, cmd
		}
	}
	return m, nil
}

func (m GoalsBoardModel) View() string {
	if m.mode == goalsModeCreate && m.form != nil {
		return m.form.View()
	}

	if !m.loaded {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(config.ColorMuted)).
			Margin(1, 2).
			Render("Loading goals…")
	}

	header := m.renderHeader()
	board := m.renderBoard()
	hint := m.renderHint()

	return lipgloss.JoinVertical(lipgloss.Left, header, "", board, "", hint)
}

// --- key handlers ---

func (m GoalsBoardModel) handleNormalKey(msg tea.KeyMsg) (GoalsBoardModel, tea.Cmd) {
	switch {
	case key.Matches(msg, Board.Up):
		if m.row > 0 {
			m.row--
		}
	case key.Matches(msg, Board.Down):
		if m.row < len(m.cols[m.col])-1 {
			m.row++
		}
	case key.Matches(msg, Board.Left):
		if m.col > 0 {
			m.col--
			m.clampCursor()
		}
	case key.Matches(msg, Board.Right):
		if m.col < 2 {
			m.col++
			m.clampCursor()
		}
	case key.Matches(msg, Board.Select):
		if m.currentGoal() != nil {
			m.mode = goalsModeSelected
		}
	case key.Matches(msg, Board.Create):
		f := newCreateForm(m.stores, m.sprint, m.width, m.height)
		m.form = &f
		m.mode = goalsModeCreate
		return m, m.form.Init()
	case key.Matches(msg, Board.Delete):
		if g := m.currentGoal(); g != nil {
			m.pendingGoal = g
			m.mode = goalsModeConfirm
		}
	}
	return m, nil
}

func (m GoalsBoardModel) handleSelectedKey(msg tea.KeyMsg) (GoalsBoardModel, tea.Cmd) {
	g := m.currentGoal()
	if g == nil {
		m.mode = goalsModeNormal
		return m, nil
	}

	switch {
	case key.Matches(msg, Board.Select):
		m.mode = goalsModeNormal
	case key.Matches(msg, Global.Back):
		m.mode = goalsModeNormal
	case key.Matches(msg, Board.Left):
		if newStatus := prevStatus(g.Status); newStatus != g.Status {
			m.mode = goalsModeNormal
			return m, m.moveGoalCmd(g, newStatus)
		}
	case key.Matches(msg, Board.Right):
		if newStatus := nextStatus(g.Status); newStatus != g.Status {
			m.mode = goalsModeNormal
			return m, m.moveGoalCmd(g, newStatus)
		}
	}
	return m, nil
}

func (m GoalsBoardModel) handleConfirmKey(msg tea.KeyMsg) (GoalsBoardModel, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.mode = goalsModeNormal
		return m, m.deleteGoalCmd(m.pendingGoal)
	case "n", "N", "esc":
		m.mode = goalsModeNormal
	}
	return m, nil
}

// --- board operations ---

func (m GoalsBoardModel) moveGoalCmd(g *models.Goal, toStatus models.Status) tea.Cmd {
	s := m.stores.Goals
	id := g.ID
	return func() tea.Msg {
		if err := s.UpdateStatus(id, toStatus); err != nil {
			return errMsg{err}
		}
		return goalsReloadMsg{}
	}
}

func (m GoalsBoardModel) deleteGoalCmd(g *models.Goal) tea.Cmd {
	s := m.stores.Goals
	id := g.ID
	return func() tea.Msg {
		if err := s.Delete(id); err != nil {
			return errMsg{err}
		}
		return goalsReloadMsg{}
	}
}

func (m GoalsBoardModel) loadCmd() tea.Cmd {
	s := m.stores.Goals
	return func() tea.Msg {
		goals, err := s.GetAll()
		if err != nil {
			return errMsg{fmt.Errorf("load goals: %w", err)}
		}
		return goalsLoadedMsg{goals}
	}
}

// --- rendering ---

func (m GoalsBoardModel) renderHeader() string {
	return lipgloss.NewStyle().Margin(0, 1).Render(
		lipgloss.NewStyle().
			Foreground(lipgloss.Color(config.ColorTitle)).
			Bold(true).
			Render("Goals Board"),
	)
}

func (m GoalsBoardModel) renderBoard() string {
	available := m.width - 2
	colWidth := available / 3
	midWidth := available - colWidth*2

	colHeight := m.height - 6
	if colHeight < 5 {
		colHeight = 5
	}

	left := m.renderCol(0, colWidth, colHeight)
	mid := m.renderCol(1, midWidth, colHeight)
	right := m.renderCol(2, colWidth, colHeight)

	return lipgloss.NewStyle().Margin(0, 1).Render(
		lipgloss.JoinHorizontal(lipgloss.Top, left, mid, right),
	)
}

func (m GoalsBoardModel) renderCol(ci, colWidth, colHeight int) string {
	goals := m.cols[ci]
	isActiveCol := ci == m.col

	borderColor := config.ColorBorder
	if isActiveCol {
		borderColor = config.ColorAccent
	}

	colStatuses := []models.Status{models.StatusTodo, models.StatusInProgress, models.StatusDone}
	colStatus := colStatuses[ci]

	label := lipgloss.NewStyle().
		Foreground(statusColor(colStatus)).
		Bold(true).
		Render(statusLabel(colStatus))
	count := lipgloss.NewStyle().
		Foreground(lipgloss.Color(config.ColorMuted)).
		Render(fmt.Sprintf("(%d)", len(goals)))
	header := label + " " + count

	innerWidth := colWidth - 2
	sep := lipgloss.NewStyle().
		Foreground(lipgloss.Color(config.ColorBorder)).
		Render(strings.Repeat("─", innerWidth))

	itemHeight := colHeight - 4
	offset := m.scrollOffset(ci, itemHeight)

	var lines []string
	lines = append(lines, header, sep)

	for ri, g := range goals {
		if ri < offset {
			continue
		}
		if len(lines)-2 >= itemHeight {
			break
		}
		isActive := isActiveCol && ri == m.row
		isSelected := m.mode == goalsModeSelected && isActive
		lines = append(lines, m.renderGoalItem(g, isActive, isSelected, innerWidth))
	}

	content := strings.Join(lines, "\n")

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(borderColor)).
		Width(colWidth).
		Height(colHeight).
		Render(content)
}

func (m GoalsBoardModel) renderGoalItem(g *models.Goal, isActive, isSelected bool, maxWidth int) string {
	bullet := lipgloss.NewStyle().
		Foreground(goalHex(g.Color)).
		Render("◆")

	idStr := lipgloss.NewStyle().
		Foreground(lipgloss.Color(config.ColorMuted)).
		Render(g.TicketID)

	pBadge := priorityBadge(g.Priority)

	// fixed cost: bullet(1) + sp(1) + id(6) + sp(1) + priority(2) = 11
	fixedCost := 1 + 1 + len([]rune(g.TicketID)) + 1 + 2
	titleWidth := maxWidth - fixedCost
	if titleWidth < 4 {
		titleWidth = 4
	}
	title := truncate(g.Title, titleWidth)

	line := bullet + " " + idStr + " " + title + pBadge

	rowStyle := lipgloss.NewStyle().Width(maxWidth)
	switch {
	case isSelected:
		rowStyle = rowStyle.
			Background(lipgloss.Color(config.ColorInProgress)).
			Foreground(lipgloss.Color(config.ColorBackground)).
			Bold(true)
	case isActive:
		rowStyle = rowStyle.
			Background(lipgloss.Color(config.ColorSurface)).
			Foreground(lipgloss.Color(config.ColorForeground))
	}

	return rowStyle.Render(line)
}

func (m GoalsBoardModel) renderHint() string {
	switch m.mode {
	case goalsModeConfirm:
		id := ""
		if m.pendingGoal != nil {
			id = m.pendingGoal.TicketID
		}
		return lipgloss.NewStyle().
			Margin(0, 1).
			Foreground(lipgloss.Color(config.ColorPriorityHigh)).
			Render(fmt.Sprintf("Delete %s and all linked non-done items? [y/N]", id))

	case goalsModeSelected:
		return lipgloss.NewStyle().Margin(0, 1).Render(hintBar([]string{
			kb(Board.Left) + " move left",
			kb(Board.Right) + " move right",
			kb(Board.Select) + " deselect",
		}))

	default:
		return lipgloss.NewStyle().Margin(0, 1).Render(hintBar([]string{
			kb(Board.Select) + " select",
			kb(Board.Create) + " create",
			kb(Board.Delete) + " delete",
			kb(Global.GoalsBoard) + " dashboard",
			kb(Global.SprintStats) + " stats",
			kb(Global.Monthly) + " monthly",
			kb(Global.Help) + " help",
			kb(Global.Back) + " quit",
		}))
	}
}

// --- helpers ---

func (m GoalsBoardModel) currentGoal() *models.Goal {
	col := m.cols[m.col]
	if len(col) == 0 || m.row >= len(col) {
		return nil
	}
	return col[m.row]
}

func (m *GoalsBoardModel) clampCursor() {
	n := len(m.cols[m.col])
	if n == 0 {
		m.row = 0
		return
	}
	if m.row >= n {
		m.row = n - 1
	}
}

func (m GoalsBoardModel) scrollOffset(ci, itemHeight int) int {
	if ci != m.col || itemHeight <= 0 {
		return 0
	}
	if m.row < itemHeight {
		return 0
	}
	return m.row - itemHeight + 1
}

// buildGoalCols organises goals into the three kanban columns by status.
// Ordering within each column follows the store's ORDER BY status, position.
func buildGoalCols(goals []*models.Goal) [3][]*models.Goal {
	var cols [3][]*models.Goal
	for _, g := range goals {
		switch g.Status {
		case models.StatusInProgress:
			cols[1] = append(cols[1], g)
		case models.StatusDone:
			cols[2] = append(cols[2], g)
		default:
			cols[0] = append(cols[0], g)
		}
	}
	return cols
}

// --- messages ---

type goalsLoadedMsg struct{ goals []*models.Goal }
type goalsReloadMsg struct{}
