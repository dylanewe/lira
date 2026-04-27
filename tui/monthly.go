package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/dylanewe/lira/config"
	"github.com/dylanewe/lira/store"
)

// MonthlyModel is the monthly analysis screen (M key).
// Shows tickets completed, sprint velocity trend, and habit streaks
// for the current calendar month.
//
// TODO: add velocity sparkline and per-sprint breakdown table.
type MonthlyModel struct {
	stores  Stores
	width   int
	height  int
	stats   *store.MonthlyStats
	loading bool
}

func newMonthlyModel(stores Stores, w, h int) MonthlyModel {
	return MonthlyModel{stores: stores, width: w, height: h, loading: true}
}

func (m MonthlyModel) Init() tea.Cmd {
	return m.loadCmd()
}

func (m MonthlyModel) Update(msg tea.Msg) (MonthlyModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case monthlyStatsMsg:
		m.stats = msg.stats
		m.loading = false
	}
	return m, nil
}

func (m MonthlyModel) View() string {
	now := time.Now()

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(config.ColorTitle)).
		Bold(true)

	mutedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(config.ColorMuted))

	hintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(config.ColorKeybind))

	accentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(config.ColorAccent))

	var b strings.Builder

	b.WriteString(titleStyle.Render("Monthly Analysis") + "  " +
		mutedStyle.Render(now.Format("January 2006")) + "\n\n")

	if m.loading {
		b.WriteString(mutedStyle.Render("Loading...") + "\n")
		return lipgloss.NewStyle().Margin(2, 4).Render(b.String())
	}

	if m.stats == nil {
		b.WriteString(mutedStyle.Render("No data yet.") + "\n")
	} else {
		b.WriteString(fmt.Sprintf("  Tickets completed:  %s\n",
			accentStyle.Render(fmt.Sprintf("%d", m.stats.TotalDone))))
		b.WriteString(fmt.Sprintf("  Habit streak:       %s\n",
			accentStyle.Render(fmt.Sprintf("%d sprint(s)", m.stats.Streak))))

		if len(m.stats.SprintVelocities) > 0 {
			b.WriteString("\n  Velocity by sprint:\n")
			for _, sv := range m.stats.SprintVelocities {
				bar := velocityBar(sv.Velocity, 20)
				b.WriteString(fmt.Sprintf("    Sprint %2d  %s  %.1f/day\n",
					sv.SprintNumber, bar, sv.Velocity))
			}
		}
	}

	b.WriteString("\n" + hintStyle.Render("m/q") + " close")

	return lipgloss.NewStyle().Margin(2, 4).Width(m.width - 8).Render(b.String())
}

// velocityBar renders a simple ASCII bar proportional to velocity.
func velocityBar(v float64, maxWidth int) string {
	const maxVelocity = 10.0
	filled := int(v / maxVelocity * float64(maxWidth))
	if filled > maxWidth {
		filled = maxWidth
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", maxWidth-filled)
	return lipgloss.NewStyle().Foreground(lipgloss.Color(config.ColorDone)).Render(bar)
}

// --- command ---

type monthlyStatsMsg struct{ stats *store.MonthlyStats }

func (m MonthlyModel) loadCmd() tea.Cmd {
	s := m.stores.Sprints
	now := time.Now()
	return func() tea.Msg {
		stats, err := s.GetMonthlyStats(now.Year(), int(now.Month()))
		if err != nil {
			return errMsg{err}
		}
		return monthlyStatsMsg{stats}
	}
}
