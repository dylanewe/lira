package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/dylanewe/lira/config"
	"github.com/dylanewe/lira/models"
	"github.com/dylanewe/lira/store"
)

// SprintStatsModel is the sprint stats overlay (Y key).
// Left/right arrow keys navigate between sprints.
//
// TODO: replace placeholder with full stats rendering.
type SprintStatsModel struct {
	stores  Stores
	sprints []*models.Sprint // all sprints, loaded on Init
	cursor  int              // index into sprints slice
	stats   *store.SprintStats
	width   int
	height  int
	loading bool
}

func newSprintStatsModel(stores Stores, active *models.Sprint, w, h int) SprintStatsModel {
	return SprintStatsModel{
		stores:  stores,
		width:   w,
		height:  h,
		loading: true,
	}
}

func (m SprintStatsModel) Init() tea.Cmd {
	return m.loadSprintsCmd()
}

func (m SprintStatsModel) Update(msg tea.Msg) (SprintStatsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case sprintsLoadedMsg:
		m.sprints = msg.sprints
		m.loading = false
		// Start on the most recent sprint.
		if len(m.sprints) > 0 {
			m.cursor = len(m.sprints) - 1
			return m, m.loadStatsCmd()
		}

	case sprintStatsLoadedMsg:
		m.stats = msg.stats

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, Stats.PrevSprint) && m.cursor > 0:
			m.cursor--
			return m, m.loadStatsCmd()
		case key.Matches(msg, Stats.NextSprint) && m.cursor < len(m.sprints)-1:
			m.cursor++
			return m, m.loadStatsCmd()
		case key.Matches(msg, Stats.Close):
			return m, func() tea.Msg { return closeStatsMsg{} }
		}
	}
	return m, nil
}

func (m SprintStatsModel) View() string {
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(config.ColorBorder)).
		Padding(1, 3).
		Width(min(m.width-8, 60))

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(config.ColorTitle)).
		Bold(true)

	mutedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(config.ColorMuted))

	hintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(config.ColorKeybind))

	var b strings.Builder

	if m.loading || len(m.sprints) == 0 {
		b.WriteString(mutedStyle.Render("Loading stats..."))
		return boxStyle.Render(b.String())
	}

	sp := m.sprints[m.cursor]
	nav := fmt.Sprintf("Sprint %d of %d", m.cursor+1, len(m.sprints))
	b.WriteString(titleStyle.Render(fmt.Sprintf("Sprint %d", sp.Number)) + "  " +
		mutedStyle.Render(nav) + "\n\n")

	b.WriteString(fmt.Sprintf("  %s  →  %s\n\n",
		sp.StartDate.Format("Jan 2"),
		sp.EndDate.Format("Jan 2, 2006"),
	))

	if m.stats != nil {
		b.WriteString(fmt.Sprintf("  Completed:   %d / %d tickets\n", m.stats.TotalDone, m.stats.TotalCreated))
		b.WriteString(fmt.Sprintf("  Velocity:    %.1f tickets/day\n", m.stats.Velocity))
		if sp.Status == models.SprintClosed {
			b.WriteString(fmt.Sprintf("  Carried out: %d tickets\n", m.stats.TotalCarriedOut))
		}
	} else {
		b.WriteString(mutedStyle.Render("  loading stats...") + "\n")
	}

	b.WriteString("\n")
	b.WriteString(hintStyle.Render("←/→") + " navigate  " +
		hintStyle.Render("y/esc") + " close")

	return boxStyle.Render(b.String())
}

// --- commands ---

type sprintsLoadedMsg struct{ sprints []*models.Sprint }
type sprintStatsLoadedMsg struct{ stats *store.SprintStats }

func (m SprintStatsModel) loadSprintsCmd() tea.Cmd {
	s := m.stores.Sprints
	return func() tea.Msg {
		sprints, err := s.GetAll()
		if err != nil {
			return errMsg{err}
		}
		return sprintsLoadedMsg{sprints}
	}
}

func (m SprintStatsModel) loadStatsCmd() tea.Cmd {
	if len(m.sprints) == 0 {
		return nil
	}
	sprintID := m.sprints[m.cursor].ID
	s := m.stores.Sprints
	return func() tea.Msg {
		stats, err := s.GetStats(sprintID)
		if err != nil {
			return errMsg{err}
		}
		return sprintStatsLoadedMsg{stats}
	}
}

