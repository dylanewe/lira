package tui

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/dylanewe/lira/config"
	"github.com/dylanewe/lira/models"
)

// ---- picker ----------------------------------------------------------------

type pickerOption struct {
	label string
	value any
}

type simplePicker struct {
	options []pickerOption
	cursor  int
}

func (p *simplePicker) up() {
	if p.cursor > 0 {
		p.cursor--
	}
}

func (p *simplePicker) down() {
	if p.cursor < len(p.options)-1 {
		p.cursor++
	}
}

func (p simplePicker) chosen() pickerOption {
	if len(p.options) == 0 {
		return pickerOption{}
	}
	return p.options[p.cursor]
}

func (p simplePicker) view(accentHex string) string {
	var b strings.Builder
	for i, opt := range p.options {
		if i == p.cursor {
			b.WriteString(
				lipgloss.NewStyle().
					Foreground(lipgloss.Color(accentHex)).
					Bold(true).
					Render("▶ "+opt.label) + "\n",
			)
		} else {
			b.WriteString(
				lipgloss.NewStyle().
					Foreground(lipgloss.Color(config.ColorSubtle)).
					Render("  "+opt.label) + "\n",
			)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// colorPicker renders picker options with a colored swatch.
func colorPickerView(p simplePicker) string {
	var b strings.Builder
	for i, opt := range p.options {
		gc, _ := opt.value.(config.GoalColor)
		swatch := lipgloss.NewStyle().
			Background(lipgloss.Color(gc.Hex)).
			Render("  ")

		cursor := "  "
		labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(config.ColorSubtle))
		if i == p.cursor {
			cursor = "▶ "
			labelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(config.ColorAccent)).
				Bold(true)
		}
		b.WriteString(cursor + swatch + " " + labelStyle.Render(opt.label) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// ---- form field enum -------------------------------------------------------

type formField int

const (
	fieldType        formField = iota
	fieldPriority              // all types
	fieldTitle                 // all types
	fieldDescription           // all types
	fieldColor                 // goal only
	fieldGoalSelect            // step (required) / task (optional, shown only if no step selected)
	fieldStepSelect            // task only (optional)
	fieldRepeatable            // task only
	fieldConfirm               // all types
)

// ---- form model ------------------------------------------------------------

type createFormModel struct {
	stores Stores
	sprint *models.Sprint
	width  int
	height int

	// wizard state: ordered list of fields and current position
	fields  []formField
	fieldIdx int

	// accumulated selections
	ticketType  string // "goal" | "step" | "task"
	priority    models.Priority
	title       string
	description string
	colorName   string
	goalID      sql.NullInt64
	stepID      sql.NullInt64
	repeatable  bool

	// UI widgets
	typePicker     simplePicker
	priorityPicker simplePicker
	colorPicker    simplePicker
	goalPicker     simplePicker
	stepPicker     simplePicker
	titleInput     textinput.Model
	descInput      textinput.Model

	// loaded lookup data
	allGoals []*models.Goal
	allSteps []*models.Step

	// validation message
	errMsg string

	// loading state (goals/steps not yet fetched)
	loading bool
}

// createFormDoneMsg signals the dashboard to close the form and optionally reload.
type createFormDoneMsg struct {
	cancelled bool
}

// formDataLoadedMsg carries goals and steps fetched during Init.
type formDataLoadedMsg struct {
	goals []*models.Goal
	steps []*models.Step
}

func newCreateForm(stores Stores, sprint *models.Sprint, w, h int) createFormModel {
	ti := textinput.New()
	ti.Placeholder = "Title (required)"
	ti.CharLimit = 120
	ti.Width = 48

	di := textinput.New()
	di.Placeholder = "Description (optional)"
	di.CharLimit = 500
	di.Width = 48

	tp := simplePicker{options: []pickerOption{
		{label: "Goal  — large objective spanning multiple sprints", value: "goal"},
		{label: "Step  — sub-task completable within 1-2 sprints", value: "step"},
		{label: "Task  — small action item", value: "task"},
	}}

	pp := simplePicker{options: []pickerOption{
		{label: "Low", value: models.PriorityLow},
		{label: "Medium", value: models.PriorityMedium},
		{label: "High", value: models.PriorityHigh},
	}, cursor: 1} // default: Medium

	cp := simplePicker{}
	for _, gc := range config.GoalColors {
		cp.options = append(cp.options, pickerOption{label: gc.Name, value: gc})
	}

	return createFormModel{
		stores:         stores,
		sprint:         sprint,
		width:          w,
		height:         h,
		fields:         []formField{fieldType}, // expanded after type chosen
		typePicker:     tp,
		priorityPicker: pp,
		colorPicker:    cp,
		titleInput:     ti,
		descInput:      di,
		loading:        true,
	}
}

func (m createFormModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.loadDataCmd())
}

func (m createFormModel) Update(msg tea.Msg) (createFormModel, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case formDataLoadedMsg:
		m.loading = false
		m.allGoals = msg.goals
		m.allSteps = msg.steps
		m.rebuildGoalPicker()
		m.rebuildStepPicker()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Forward to active text input.
	if m.currentField() == fieldTitle {
		var cmd tea.Cmd
		m.titleInput, cmd = m.titleInput.Update(msg)
		return m, cmd
	}
	if m.currentField() == fieldDescription {
		var cmd tea.Cmd
		m.descInput, cmd = m.descInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m createFormModel) View() string {
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(config.ColorAccent)).
		Padding(1, 3).
		Width(min(m.width-8, 62))

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(config.ColorTitle)).
		Bold(true)

	stepStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(config.ColorMuted))

	errStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(config.ColorPriorityHigh))

	hintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(config.ColorKeybind))

	mutedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(config.ColorMuted))

	total := len(m.fields)
	current := m.fieldIdx + 1
	progress := stepStyle.Render(fmt.Sprintf("Step %d of %d", current, total))

	var body strings.Builder

	if m.loading {
		body.WriteString(mutedStyle.Render("Loading…"))
	} else {
		switch m.currentField() {
		case fieldType:
			body.WriteString("Select ticket type:\n\n")
			body.WriteString(m.typePicker.view(config.ColorAccent))

		case fieldPriority:
			body.WriteString("Select priority:\n\n")
			body.WriteString(m.priorityPicker.view(config.ColorAccent))

		case fieldTitle:
			body.WriteString("Enter title:\n\n")
			body.WriteString(m.titleInput.View())
			if m.errMsg != "" {
				body.WriteString("\n" + errStyle.Render(m.errMsg))
			}

		case fieldDescription:
			body.WriteString("Enter description (optional):\n\n")
			body.WriteString(m.descInput.View())

		case fieldColor:
			body.WriteString("Choose a color for this goal:\n\n")
			body.WriteString(colorPickerView(m.colorPicker))

		case fieldGoalSelect:
			required := m.ticketType == "step"
			label := "Link to a Goal:"
			if !required {
				label = "Link to a Goal (optional):"
			}
			body.WriteString(label + "\n\n")
			if len(m.goalPicker.options) == 0 {
				body.WriteString(mutedStyle.Render("No goals yet. Create one first."))
			} else {
				body.WriteString(m.goalPicker.view(config.ColorAccent))
			}

		case fieldStepSelect:
			body.WriteString("Link to a Step (optional):\n\n")
			if len(m.stepPicker.options) == 0 {
				body.WriteString(mutedStyle.Render("No steps in this sprint yet."))
			} else {
				body.WriteString(m.stepPicker.view(config.ColorAccent))
			}

		case fieldRepeatable:
			body.WriteString("Repeatable task?\n\n")
			body.WriteString("Repeatable tasks are re-created fresh each sprint.\n\n")
			on := "  ○ No"
			off := "  ○ Yes"
			if m.repeatable {
				on = lipgloss.NewStyle().Foreground(lipgloss.Color(config.ColorAccent)).Bold(true).Render("▶ ● Yes")
				off = "  ○ No"
			} else {
				off = lipgloss.NewStyle().Foreground(lipgloss.Color(config.ColorAccent)).Bold(true).Render("▶ ● No")
				on = "  ○ Yes"
			}
			body.WriteString(off + "\n" + on)

		case fieldConfirm:
			body.WriteString("Ready to create:\n\n")
			body.WriteString(m.confirmSummary())
			if m.errMsg != "" {
				body.WriteString("\n" + errStyle.Render(m.errMsg))
			}
		}
	}

	hints := hintStyle.Render("enter") + mutedStyle.Render(" next  ") +
		hintStyle.Render("shift+tab") + mutedStyle.Render(" back  ") +
		hintStyle.Render("esc") + mutedStyle.Render(" cancel")

	content := titleStyle.Render("Create Ticket") + "  " + progress +
		"\n\n" + body.String() + "\n\n" + hints

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		boxStyle.Render(content),
	)
}

// ---- key handling ----------------------------------------------------------

func (m createFormModel) handleKey(msg tea.KeyMsg) (createFormModel, tea.Cmd) {
	switch {
	case key.Matches(msg, Form.Cancel):
		return m, func() tea.Msg { return createFormDoneMsg{cancelled: true} }

	case key.Matches(msg, Form.Prev):
		if m.fieldIdx > 0 {
			m.fieldIdx--
			m.errMsg = ""
			return m.focusCurrentField()
		}
		return m, nil

	case key.Matches(msg, Form.Confirm):
		return m.advance()

	case key.Matches(msg, Board.Up):
		m.pickerUp()
	case key.Matches(msg, Board.Down):
		m.pickerDown()

	case key.Matches(msg, Form.Toggle):
		if m.currentField() == fieldRepeatable {
			m.repeatable = !m.repeatable
		}
	}

	// Forward keystrokes to the active text input.
	if m.currentField() == fieldTitle {
		var cmd tea.Cmd
		m.titleInput, cmd = m.titleInput.Update(msg)
		return m, cmd
	}
	if m.currentField() == fieldDescription {
		var cmd tea.Cmd
		m.descInput, cmd = m.descInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

// advance validates the current field and moves to the next one.
// On the confirm step it submits the form.
func (m createFormModel) advance() (createFormModel, tea.Cmd) {
	m.errMsg = ""

	switch m.currentField() {
	case fieldType:
		m.ticketType = m.typePicker.chosen().value.(string)
		m.fields = fieldOrderFor(m.ticketType)
		m.fieldIdx = 1
		return m.focusCurrentField()

	case fieldPriority:
		m.priority = m.priorityPicker.chosen().value.(models.Priority)

	case fieldTitle:
		m.title = strings.TrimSpace(m.titleInput.Value())
		if m.title == "" {
			m.errMsg = "Title is required."
			return m, nil
		}

	case fieldDescription:
		m.description = strings.TrimSpace(m.descInput.Value())

	case fieldColor:
		gc := m.colorPicker.chosen().value.(config.GoalColor)
		m.colorName = gc.Name

	case fieldGoalSelect:
		opt := m.goalPicker.chosen()
		if opt.value == nil {
			m.goalID = sql.NullInt64{}
		} else {
			g := opt.value.(*models.Goal)
			m.goalID = sql.NullInt64{Int64: g.ID, Valid: true}
		}
		// For steps, goal is required.
		if m.ticketType == "step" && !m.goalID.Valid {
			m.errMsg = "A Goal is required for Steps."
			return m, nil
		}

	case fieldStepSelect:
		opt := m.stepPicker.chosen()
		if opt.value == nil {
			m.stepID = sql.NullInt64{}
		} else {
			s := opt.value.(*models.Step)
			m.stepID = sql.NullInt64{Int64: s.ID, Valid: true}
			// Inherit the step's goal.
			m.goalID = sql.NullInt64{Int64: s.GoalID, Valid: true}
		}
		// If a step was chosen, skip fieldGoalSelect.
		if m.stepID.Valid {
			next := m.fieldIdx + 1
			for next < len(m.fields) && m.fields[next] == fieldGoalSelect {
				next++
			}
			m.fieldIdx = next
			return m.focusCurrentField()
		}

	case fieldRepeatable:
		// value already toggled via space; just advance

	case fieldConfirm:
		return m, m.submitCmd()
	}

	m.fieldIdx++
	return m.focusCurrentField()
}

// ---- picker navigation -----------------------------------------------------

func (m *createFormModel) pickerUp() {
	switch m.currentField() {
	case fieldType:
		m.typePicker.up()
	case fieldPriority:
		m.priorityPicker.up()
	case fieldColor:
		m.colorPicker.up()
	case fieldGoalSelect:
		m.goalPicker.up()
	case fieldStepSelect:
		m.stepPicker.up()
	case fieldRepeatable:
		m.repeatable = !m.repeatable
	}
}

func (m *createFormModel) pickerDown() {
	switch m.currentField() {
	case fieldType:
		m.typePicker.down()
	case fieldPriority:
		m.priorityPicker.down()
	case fieldColor:
		m.colorPicker.down()
	case fieldGoalSelect:
		m.goalPicker.down()
	case fieldStepSelect:
		m.stepPicker.down()
	case fieldRepeatable:
		m.repeatable = !m.repeatable
	}
}

// focusCurrentField blurs all inputs then focuses the active one.
func (m createFormModel) focusCurrentField() (createFormModel, tea.Cmd) {
	m.titleInput.Blur()
	m.descInput.Blur()
	switch m.currentField() {
	case fieldTitle:
		cmd := m.titleInput.Focus()
		return m, cmd
	case fieldDescription:
		cmd := m.descInput.Focus()
		return m, cmd
	}
	return m, nil
}

// ---- helpers ---------------------------------------------------------------

func (m createFormModel) currentField() formField {
	if m.fieldIdx >= len(m.fields) {
		return fieldConfirm
	}
	return m.fields[m.fieldIdx]
}

// fieldOrderFor returns the wizard step sequence for the chosen ticket type.
func fieldOrderFor(t string) []formField {
	switch t {
	case "goal":
		return []formField{
			fieldType, fieldPriority, fieldTitle, fieldDescription,
			fieldColor, fieldConfirm,
		}
	case "step":
		return []formField{
			fieldType, fieldPriority, fieldTitle, fieldDescription,
			fieldGoalSelect, fieldConfirm,
		}
	default: // task
		return []formField{
			fieldType, fieldPriority, fieldTitle, fieldDescription,
			fieldStepSelect, fieldGoalSelect, fieldRepeatable, fieldConfirm,
		}
	}
}

func (m *createFormModel) rebuildGoalPicker() {
	opts := []pickerOption{{label: "None", value: nil}}
	for _, g := range m.allGoals {
		gc, _ := config.GoalColorByName(g.Color)
		swatch := lipgloss.NewStyle().
			Foreground(lipgloss.Color(gc.Hex)).
			Render("■ ")
		opts = append(opts, pickerOption{
			label: swatch + g.TicketID + " " + truncate(g.Title, 40),
			value: g,
		})
	}
	m.goalPicker = simplePicker{options: opts}
}

func (m *createFormModel) rebuildStepPicker() {
	opts := []pickerOption{{label: "None", value: nil}}
	for _, s := range m.allSteps {
		opts = append(opts, pickerOption{
			label: s.TicketID + " " + truncate(s.Title, 40),
			value: s,
		})
	}
	m.stepPicker = simplePicker{options: opts}
}

// confirmSummary returns a formatted preview of the ticket about to be created.
func (m createFormModel) confirmSummary() string {
	kv := func(k, v string) string {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(config.ColorMuted)).Render(k+": ") +
			lipgloss.NewStyle().Foreground(lipgloss.Color(config.ColorForeground)).Render(v) + "\n"
	}

	var b strings.Builder
	b.WriteString(kv("Type", m.ticketType))
	b.WriteString(kv("Title", m.title))
	if m.description != "" {
		b.WriteString(kv("Description", truncate(m.description, 50)))
	}
	b.WriteString(kv("Priority", string(m.priority)))
	if m.ticketType == "goal" && m.colorName != "" {
		b.WriteString(kv("Color", m.colorName))
	}
	if m.goalID.Valid {
		for _, g := range m.allGoals {
			if g.ID == m.goalID.Int64 {
				b.WriteString(kv("Goal", g.TicketID+" "+g.Title))
				break
			}
		}
	}
	if m.stepID.Valid {
		for _, s := range m.allSteps {
			if s.ID == m.stepID.Int64 {
				b.WriteString(kv("Step", s.TicketID+" "+s.Title))
				break
			}
		}
	}
	if m.ticketType == "task" && m.repeatable {
		b.WriteString(kv("Repeatable", "yes"))
	}
	return b.String()
}

// ---- commands --------------------------------------------------------------

func (m createFormModel) loadDataCmd() tea.Cmd {
	stores := m.stores
	sprint := m.sprint
	return func() tea.Msg {
		goals, err := stores.Goals.GetAll()
		if err != nil {
			return errMsg{fmt.Errorf("load goals: %w", err)}
		}
		var steps []*models.Step
		if sprint != nil {
			steps, err = stores.Steps.GetBySprintID(sprint.ID)
			if err != nil {
				return errMsg{fmt.Errorf("load steps: %w", err)}
			}
		}
		return formDataLoadedMsg{goals: goals, steps: steps}
	}
}

func (m createFormModel) submitCmd() tea.Cmd {
	stores := m.stores
	sprint := m.sprint

	ticketType := m.ticketType
	title := m.title
	desc := m.description
	priority := m.priority
	colorName := m.colorName
	goalID := m.goalID
	stepID := m.stepID
	repeatable := m.repeatable
	allGoals := m.allGoals

	return func() tea.Msg {
		switch ticketType {
		case "goal":
			// Auto-assign color if not picked (shouldn't happen, but be safe).
			if colorName == "" {
				colorName = config.NextAutoColor(len(allGoals)).Name
			}
			g := &models.Goal{
				Title:       title,
				Description: desc,
				Priority:    priority,
				Status:      models.StatusTodo,
				Color:       colorName,
			}
			if err := stores.Goals.Create(g); err != nil {
				return errMsg{fmt.Errorf("create goal: %w", err)}
			}

		case "step":
			if sprint == nil {
				return errMsg{fmt.Errorf("no active sprint")}
			}
			s := &models.Step{
				Title:       title,
				Description: desc,
				Priority:    priority,
				Status:      models.StatusTodo,
				GoalID:      goalID.Int64,
				SprintID:    sprint.ID,
			}
			if err := stores.Steps.Create(s); err != nil {
				return errMsg{fmt.Errorf("create step: %w", err)}
			}

		default: // task
			if sprint == nil {
				return errMsg{fmt.Errorf("no active sprint")}
			}
			t := &models.Task{
				Title:       title,
				Description: desc,
				Priority:    priority,
				Status:      models.StatusTodo,
				StepID:      stepID,
				GoalID:      goalID,
				SprintID:    sprint.ID,
				Repeatable:  repeatable,
			}
			if err := stores.Tasks.Create(t); err != nil {
				return errMsg{fmt.Errorf("create task: %w", err)}
			}
		}

		return createFormDoneMsg{cancelled: false}
	}
}
