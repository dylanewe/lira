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

// dashMode is the current interaction state of the dashboard.
type dashMode int

const (
	modeNormal   dashMode = iota
	modeSelected          // item chosen, waiting for column-move key
	modeConfirm           // delete confirmation prompt active
	modeCreate            // create-ticket form overlay
)

// DashboardModel is the main kanban board showing Steps and Tasks for the
// active sprint. Steps are top-level entries; Tasks linked to a Step appear
// beneath their parent with a ↳ prefix in the same column.
type DashboardModel struct {
	stores Stores
	sprint *models.Sprint
	width  int
	height int

	// board data (populated after load)
	cols   [3]boardCol
	loaded bool

	// cursor: which column (0-2) and which row within that column
	col int
	row int

	// interaction mode
	mode dashMode

	// modeConfirm: the item awaiting deletion
	pendingItem boardItem

	// modeCreate: the form overlay
	form *createFormModel
}

func newDashboardModel(stores Stores, sprint *models.Sprint, w, h int) DashboardModel {
	return DashboardModel{
		stores: stores,
		sprint: sprint,
		width:  w,
		height: h,
	}
}

// --- tea.Model ---

func (m DashboardModel) Init() tea.Cmd {
	return m.loadCmd()
}

func (m DashboardModel) Update(msg tea.Msg) (DashboardModel, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.form != nil {
			m.form.width, m.form.height = msg.Width, msg.Height
		}

	case boardLoadedMsg:
		m.cols = buildCols(msg.steps, msg.tasks)
		m.loaded = true
		m.clampCursor()

	case boardReloadMsg:
		return m, m.loadCmd()

	case createFormDoneMsg:
		m.form = nil
		m.mode = modeNormal
		if !msg.cancelled {
			return m, m.loadCmd()
		}

	case tea.KeyMsg:
		switch m.mode {
		case modeCreate:
			if m.form != nil {
				updated, cmd := m.form.Update(msg)
				m.form = &updated
				return m, cmd
			}
		case modeConfirm:
			return m.handleConfirmKey(msg)
		case modeSelected:
			return m.handleSelectedKey(msg)
		default:
			return m.handleNormalKey(msg)
		}
	default:
		if m.mode == modeCreate && m.form != nil {
			updated, cmd := m.form.Update(msg)
			m.form = &updated
			return m, cmd
		}
	}

	return m, nil
}

func (m DashboardModel) View() string {
	if !m.loaded {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(config.ColorMuted)).
			Margin(1, 2).
			Render("Loading sprint data…")
	}

	// Create form overlays the entire screen.
	if m.mode == modeCreate && m.form != nil {
		return m.form.View()
	}

	header := m.renderHeader()
	board := m.renderBoard()
	hint := m.renderHint()

	return lipgloss.JoinVertical(lipgloss.Left, header, "", board, "", hint)
}

// --- key handlers ---

func (m DashboardModel) handleNormalKey(msg tea.KeyMsg) (DashboardModel, tea.Cmd) {
	switch {
	case key.Matches(msg, Board.Up):
		if m.row > 0 {
			m.row--
		}
	case key.Matches(msg, Board.Down):
		if m.row < len(m.cols[m.col].items)-1 {
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
		if m.currentItem() != nil {
			m.mode = modeSelected
		}
	case key.Matches(msg, Board.Create):
		f := newCreateForm(m.stores, m.sprint, m.width, m.height)
		m.form = &f
		m.mode = modeCreate
		return m, m.form.Init()
	case key.Matches(msg, Board.Delete):
		item := m.currentItem()
		if item != nil {
			m.pendingItem = *item
			m.mode = modeConfirm
		}
	}
	return m, nil
}

func (m DashboardModel) handleSelectedKey(msg tea.KeyMsg) (DashboardModel, tea.Cmd) {
	item := m.currentItem()
	if item == nil {
		m.mode = modeNormal
		return m, nil
	}

	switch {
	case key.Matches(msg, Board.Select): // deselect
		m.mode = modeNormal

	case key.Matches(msg, Global.Back): // esc deselects
		m.mode = modeNormal

	case key.Matches(msg, Board.Left):
		newStatus := prevStatus(item.status())
		if newStatus != item.status() {
			m.mode = modeNormal
			return m, m.moveItemCmd(*item, newStatus)
		}

	case key.Matches(msg, Board.Right):
		newStatus := nextStatus(item.status())
		if newStatus != item.status() {
			m.mode = modeNormal
			return m, m.moveItemCmd(*item, newStatus)
		}
	}
	return m, nil
}

func (m DashboardModel) handleConfirmKey(msg tea.KeyMsg) (DashboardModel, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.mode = modeNormal
		return m, m.deleteItemCmd(m.pendingItem)
	case "n", "N", "esc":
		m.mode = modeNormal
	}
	return m, nil
}

// --- board operations ---

func (m DashboardModel) moveItemCmd(item boardItem, toStatus models.Status) tea.Cmd {
	stores := m.stores
	return func() tea.Msg {
		var err error
		if item.kind == kindStep {
			err = stores.Steps.UpdateStatus(item.dbID(), toStatus)
		} else {
			err = stores.Tasks.UpdateStatus(item.dbID(), toStatus)
		}
		if err != nil {
			return errMsg{err}
		}
		return boardReloadMsg{}
	}
}

func (m DashboardModel) deleteItemCmd(item boardItem) tea.Cmd {
	stores := m.stores
	return func() tea.Msg {
		var err error
		if item.kind == kindStep {
			err = stores.Steps.Delete(item.dbID())
		} else {
			err = stores.Tasks.Delete(item.dbID())
		}
		if err != nil {
			return errMsg{err}
		}
		return boardReloadMsg{}
	}
}

func (m DashboardModel) loadCmd() tea.Cmd {
	stores := m.stores
	sprint := m.sprint
	return func() tea.Msg {
		if sprint == nil {
			return boardLoadedMsg{}
		}
		steps, err := stores.Steps.GetBySprintID(sprint.ID)
		if err != nil {
			return errMsg{fmt.Errorf("load steps: %w", err)}
		}
		tasks, err := stores.Tasks.GetBySprintID(sprint.ID)
		if err != nil {
			return errMsg{fmt.Errorf("load tasks: %w", err)}
		}
		return boardLoadedMsg{steps: steps, tasks: tasks}
	}
}

// --- rendering ---

func (m DashboardModel) renderHeader() string {
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(config.ColorTitle)).
		Bold(true)

	sprintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(config.ColorAccent))

	sprintLabel := ""
	if m.sprint != nil {
		sprintLabel = sprintStyle.Render(fmt.Sprintf("Sprint %d", m.sprint.Number)) +
			lipgloss.NewStyle().Foreground(lipgloss.Color(config.ColorMuted)).
				Render(fmt.Sprintf("  ends %s", m.sprint.EndDate.Format("Jan 2")))
	}

	return lipgloss.NewStyle().Margin(0, 1).Render(
		titleStyle.Render("Dashboard") + "  " + sprintLabel,
	)
}

func (m DashboardModel) renderBoard() string {
	// Distribute available width across the 3 columns.
	// Each column gets an equal share; the middle column absorbs any remainder.
	available := m.width - 2 // outer margin
	colWidth := available / 3
	midWidth := available - colWidth*2

	// Reserve rows for header (2) + gap (1) + hint (2) + gap (1) = 6
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

func (m DashboardModel) renderCol(ci, colWidth, colHeight int) string {
	col := m.cols[ci]
	isActiveCol := ci == m.col

	borderColor := config.ColorBorder
	if isActiveCol {
		borderColor = config.ColorAccent
	}

	// Column header
	label := lipgloss.NewStyle().
		Foreground(statusColor(col.status)).
		Bold(true).
		Render(statusLabel(col.status))
	count := lipgloss.NewStyle().
		Foreground(lipgloss.Color(config.ColorMuted)).
		Render(fmt.Sprintf("(%d)", len(col.items)))
	header := label + " " + count

	// Separator
	innerWidth := colWidth - 2 // subtract border chars
	sep := lipgloss.NewStyle().
		Foreground(lipgloss.Color(config.ColorBorder)).
		Render(strings.Repeat("─", innerWidth))

	// Items — visible window for scrolling
	itemHeight := colHeight - 4 // border(2) + header(1) + sep(1)
	offset := m.scrollOffset(ci, itemHeight)

	var lines []string
	lines = append(lines, header, sep)

	for ri, item := range col.items {
		if ri < offset {
			continue
		}
		if len(lines)-2 >= itemHeight { // -2 for header+sep
			break
		}
		isActive := isActiveCol && ri == m.row
		isSelected := m.mode == modeSelected && isActive
		lines = append(lines, m.renderItem(item, isActive, isSelected, innerWidth))
	}

	content := strings.Join(lines, "\n")

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(borderColor)).
		Width(colWidth).
		Height(colHeight)

	return boxStyle.Render(content)
}

func (m DashboardModel) renderItem(item boardItem, isActive, isSelected bool, maxWidth int) string {
	// Prefix: indent for child tasks.
	prefix := ""
	childPrefix := ""
	if item.isChild {
		prefix = "  "
		childPrefix = "↳ "
	}

	// Bullet: ● for steps, ○ for tasks.
	bulletRune := "○"
	if item.kind == kindStep {
		bulletRune = "●"
	}
	bullet := lipgloss.NewStyle().
		Foreground(goalHex(item.colorName())).
		Render(bulletRune)

	// Ticket ID
	idStr := lipgloss.NewStyle().
		Foreground(lipgloss.Color(config.ColorMuted)).
		Render(item.id())

	// Badges (right-aligned): priority (2) + maybe repeat (1) = up to 3 chars
	pBadge := priorityBadge(item.priority())
	rBadge := " "
	if item.repeatable() {
		rBadge = lipgloss.NewStyle().
			Foreground(lipgloss.Color(config.ColorKeybind)).
			Render("↺")
	}
	badges := rBadge + pBadge

	// Fixed cost: prefix(0-2) + childPrefix(0-2) + bullet(1) + sp(1) + id(7) + sp(1) + badges(3)
	fixedCost := len([]rune(prefix)) + len([]rune(childPrefix)) + 1 + 1 + 7 + 1 + 3
	titleWidth := maxWidth - fixedCost
	if titleWidth < 4 {
		titleWidth = 4
	}
	title := truncate(item.title(), titleWidth)

	line := prefix + childPrefix + bullet + " " + idStr + " " + title + badges

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

func (m DashboardModel) renderHint() string {
	var hints []string

	switch m.mode {
	case modeConfirm:
		return lipgloss.NewStyle().
			Margin(0, 1).
			Foreground(lipgloss.Color(config.ColorPriorityHigh)).
			Render(fmt.Sprintf("Delete %s? [y/N]", m.pendingItem.id()))

	case modeSelected:
		hints = []string{
			kb(Board.Left) + " move left",
			kb(Board.Right) + " move right",
			kb(Board.Select) + " deselect",
		}

	default:
		hints = []string{
			kb(Board.Select) + " select",
			kb(Board.Create) + " create",
			kb(Board.Delete) + " delete",
			kb(Global.GoalsBoard) + " goals",
			kb(Global.SprintStats) + " stats",
			kb(Global.Monthly) + " monthly",
			kb(Global.Help) + " help",
			kb(Global.Back) + " quit",
		}
	}

	return lipgloss.NewStyle().Margin(0, 1).Render(hintBar(hints))
}

// --- helpers ---

// kb returns the styled key-label string for a binding (avoids shadowing the
// "key" package import).
func kb(b key.Binding) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(config.ColorKeybind)).
		Render(b.Help().Key)
}

// currentItem returns a pointer to the item under the cursor, or nil.
func (m DashboardModel) currentItem() *boardItem {
	col := m.cols[m.col]
	if len(col.items) == 0 || m.row >= len(col.items) {
		return nil
	}
	item := col.items[m.row]
	return &item
}

// clampCursor ensures m.row is within the bounds of the active column.
func (m *DashboardModel) clampCursor() {
	n := len(m.cols[m.col].items)
	if n == 0 {
		m.row = 0
		return
	}
	if m.row >= n {
		m.row = n - 1
	}
}

// scrollOffset computes the first item index to display so the cursor is
// always visible within the available item height.
func (m DashboardModel) scrollOffset(ci, itemHeight int) int {
	if ci != m.col || itemHeight <= 0 {
		return 0
	}
	if m.row < itemHeight {
		return 0
	}
	return m.row - itemHeight + 1
}

func nextStatus(s models.Status) models.Status {
	switch s {
	case models.StatusTodo:
		return models.StatusInProgress
	case models.StatusInProgress:
		return models.StatusDone
	default:
		return s
	}
}

func prevStatus(s models.Status) models.Status {
	switch s {
	case models.StatusDone:
		return models.StatusInProgress
	case models.StatusInProgress:
		return models.StatusTodo
	default:
		return s
	}
}

// --- messages ---

type boardLoadedMsg struct {
	steps []*models.Step
	tasks []*models.Task
}

type boardReloadMsg struct{}

