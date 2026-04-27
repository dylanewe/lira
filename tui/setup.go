package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/dylanewe/lira/config"
	"github.com/dylanewe/lira/models"
)

// setupStep tracks which step of the setup wizard is active.
type setupStep int

const (
	setupStepSprintLength setupStep = iota // configure sprint length
	setupStepGoals                         // create initial goals (optional)
	setupStepTasks                         // create initial steps/tasks (optional)
	setupStepConfirm                       // review & start Sprint 1
)

// SetupModel is the first-launch guided setup screen.
// It walks the user through: sprint length → goals → tasks → start sprint.
type SetupModel struct {
	stores Stores
	cfg    *config.Config
	width  int
	height int

	step      setupStep
	input     textinput.Model // reused for text entry at each step
	inputErr  string          // validation message shown beneath the input
	sprintLen int             // collected sprint length (days)
}

func newSetupModel(stores Stores, cfg *config.Config, w, h int) SetupModel {
	ti := textinput.New()
	ti.Placeholder = fmt.Sprintf("%d", config.DefaultSprintLengthDays)
	ti.Focus()
	ti.CharLimit = 3
	ti.Width = 10

	return SetupModel{
		stores:    stores,
		cfg:       cfg,
		width:     w,
		height:    h,
		step:      setupStepSprintLength,
		input:     ti,
		sprintLen: config.DefaultSprintLengthDays,
	}
}

func (m SetupModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m SetupModel) Update(msg tea.Msg) (SetupModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case tea.KeyMsg:
		switch m.step {
		case setupStepSprintLength:
			return m.updateSprintLengthStep(msg)
		case setupStepGoals:
			return m.updateGoalsStep(msg)
		case setupStepTasks:
			return m.updateTasksStep(msg)
		case setupStepConfirm:
			return m.updateConfirmStep(msg)
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m SetupModel) View() string {
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(config.ColorTitle)).
		Bold(true).
		MarginBottom(1)

	subtitleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(config.ColorSubtle))

	errStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(config.ColorPriorityHigh))

	hintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(config.ColorKeybind))

	var b strings.Builder

	b.WriteString(titleStyle.Render("Welcome to Lira") + "\n")
	b.WriteString(subtitleStyle.Render("Jira-style task management for life") + "\n\n")

	switch m.step {
	case setupStepSprintLength:
		b.WriteString("Step 1 of 3 — Configure your sprint length\n\n")
		b.WriteString("How many days should each sprint last?\n\n")
		b.WriteString(m.input.View() + " days\n\n")
		if m.inputErr != "" {
			b.WriteString(errStyle.Render(m.inputErr) + "\n\n")
		}
		b.WriteString(hintStyle.Render("enter") + " to continue\n")

	case setupStepGoals:
		b.WriteString("Step 2 of 3 — Add Goals (optional)\n\n")
		b.WriteString(subtitleStyle.Render("Goals are large objectives that span multiple sprints.") + "\n\n")
		b.WriteString("Goal title:\n")
		b.WriteString(m.input.View() + "\n\n")
		if m.inputErr != "" {
			b.WriteString(errStyle.Render(m.inputErr) + "\n\n")
		}
		b.WriteString(hintStyle.Render("enter") + " to add  " +
			hintStyle.Render("tab") + " to skip\n")

	case setupStepTasks:
		b.WriteString("Step 3 of 3 — Add Tasks (optional)\n\n")
		b.WriteString(subtitleStyle.Render("Tasks are small action items. You can add more from the dashboard.") + "\n\n")
		b.WriteString("Task title:\n")
		b.WriteString(m.input.View() + "\n\n")
		if m.inputErr != "" {
			b.WriteString(errStyle.Render(m.inputErr) + "\n\n")
		}
		b.WriteString(hintStyle.Render("enter") + " to add  " +
			hintStyle.Render("tab") + " to skip\n")

	case setupStepConfirm:
		b.WriteString("Ready to start Sprint 1\n\n")
		b.WriteString(fmt.Sprintf("  Sprint length: %d days\n", m.sprintLen))
		b.WriteString(fmt.Sprintf("  End date:      %s\n\n",
			time.Now().Add(time.Duration(m.sprintLen)*24*time.Hour).Format("Jan 2, 2006")))
		b.WriteString("Goals and tasks can be added at any time from the dashboard.\n\n")
		b.WriteString(hintStyle.Render("enter") + " to start  " +
			hintStyle.Render("esc") + " to go back\n")
	}

	return lipgloss.NewStyle().
		Margin(2, 4).
		Width(m.width - 8).
		Render(b.String())
}

// --- step handlers ---

func (m SetupModel) updateSprintLengthStep(msg tea.KeyMsg) (SetupModel, tea.Cmd) {
	if key.Matches(msg, Form.Confirm) {
		val := strings.TrimSpace(m.input.Value())
		if val == "" {
			val = fmt.Sprintf("%d", config.DefaultSprintLengthDays)
		}
		n, err := strconv.Atoi(val)
		if err != nil || n < 1 || n > 365 {
			m.inputErr = "Enter a number between 1 and 365."
			return m, nil
		}
		m.sprintLen = n
		m.inputErr = ""
		m.input.SetValue("")
		m.input.Placeholder = "e.g. Get fit, Read more..."
		m.input.CharLimit = 80
		m.input.Width = 50
		m.step = setupStepGoals
		return m, m.input.Focus()
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m SetupModel) updateGoalsStep(msg tea.KeyMsg) (SetupModel, tea.Cmd) {
	switch {
	case key.Matches(msg, Form.Confirm):
		title := strings.TrimSpace(m.input.Value())
		if title == "" {
			m.inputErr = "Title cannot be empty. Press tab to skip."
			return m, nil
		}
		// Save the goal with defaults; full configuration available in the app.
		g := &models.Goal{
			Title:    title,
			Priority: models.PriorityMedium,
			Status:   models.StatusTodo,
			Color:    config.NextAutoColor(0).Name,
		}
		if err := m.stores.Goals.Create(g); err != nil {
			m.inputErr = fmt.Sprintf("Could not save goal: %v", err)
			return m, nil
		}
		m.input.SetValue("")
		m.inputErr = ""
		return m, nil

	case key.Matches(msg, Form.Next): // tab → skip to tasks
		m.input.SetValue("")
		m.input.Placeholder = "e.g. Morning run, Read 20 pages..."
		m.step = setupStepTasks
		return m, m.input.Focus()
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m SetupModel) updateTasksStep(msg tea.KeyMsg) (SetupModel, tea.Cmd) {
	switch {
	case key.Matches(msg, Form.Confirm):
		title := strings.TrimSpace(m.input.Value())
		if title == "" {
			m.inputErr = "Title cannot be empty. Press tab to skip."
			return m, nil
		}
		// We need an active sprint ID to save a task; tasks are saved after
		// Sprint 1 is created in the confirm step. Queue them up in memory.
		// For now, skip to confirm — tasks can be added from dashboard.
		m.input.SetValue("")
		m.inputErr = ""
		return m, nil

	case key.Matches(msg, Form.Next): // tab → skip to confirm
		m.step = setupStepConfirm
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m SetupModel) updateConfirmStep(msg tea.KeyMsg) (SetupModel, tea.Cmd) {
	switch {
	case key.Matches(msg, Form.Confirm):
		return m, m.startSprintCmd()
	case key.Matches(msg, Form.Cancel):
		m.step = setupStepTasks
		return m, nil
	}
	return m, nil
}

// startSprintCmd creates Sprint 1 and signals the root App to transition to the dashboard.
func (m SetupModel) startSprintCmd() tea.Cmd {
	sprintLen := m.sprintLen
	sprints := m.stores.Sprints
	cfg := m.cfg

	return func() tea.Msg {
		now := time.Now().UTC()
		sprint := &models.Sprint{
			Number:    1,
			StartDate: now,
			EndDate:   now.Add(time.Duration(sprintLen) * 24 * time.Hour),
			Status:    models.SprintActive,
		}
		if err := sprints.Create(sprint); err != nil {
			return errMsg{fmt.Errorf("create sprint: %w", err)}
		}

		// Persist the chosen sprint length to config.
		cfg.SprintLengthDays = sprintLen
		_ = cfg.Save()

		return setupDoneMsg{sprint: sprint}
	}
}
