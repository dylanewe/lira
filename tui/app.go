package tui

import (
	"database/sql"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"

	"github.com/dylanewe/lira/config"
	liradb "github.com/dylanewe/lira/db"
	"github.com/dylanewe/lira/models"
	"github.com/dylanewe/lira/store"
)

// activeView tracks which screen is currently rendered.
type activeView int

const (
	viewLoading    activeView = iota
	viewSetup                 // first-launch guided setup
	viewDashboard             // main kanban (steps + tasks)
	viewGoalsBoard            // goals-only kanban
	viewSprintStats           // sprint stats overlay
	viewMonthly               // monthly analysis screen
	viewError                 // unrecoverable startup error
)

// Stores bundles all data-access objects so sub-models can query the DB.
type Stores struct {
	Goals   *store.GoalStore
	Steps   *store.StepStore
	Tasks   *store.TaskStore
	Sprints *store.SprintStore
}

// App is the root Bubbletea model. It owns startup, view routing, and terminal
// resize events. All sub-models are initialized after startup completes.
type App struct {
	// infrastructure (nil until appReadyMsg received)
	db     *sql.DB
	cfg    *config.Config
	stores Stores

	// current sprint (nil until ready)
	sprint *models.Sprint

	// routing
	current activeView
	prev    activeView // used by overlays to restore the view behind them

	// sub-models (non-nil once navigated to)
	setup       *SetupModel
	dashboard   *DashboardModel
	goalsBoard  *GoalsBoardModel
	sprintStats *SprintStatsModel
	monthly     *MonthlyModel

	// terminal dimensions
	width  int
	height int

	// startup error (shown on viewError)
	err error
}

// New returns the initial App model. The DB is not yet open — Init() triggers
// the async startup sequence via a command.
func New() App {
	return App{current: viewLoading}
}

// --- tea.Model interface ---

func (a App) Init() tea.Cmd {
	return startupCmd
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		return a, a.resizeActive()

	case appReadyMsg:
		a.db = msg.db
		a.cfg = msg.cfg
		a.stores = Stores{
			Goals:   store.NewGoalStore(msg.db),
			Steps:   store.NewStepStore(msg.db),
			Tasks:   store.NewTaskStore(msg.db),
			Sprints: store.NewSprintStore(msg.db),
		}
		a.sprint = msg.sprint

		if msg.isFirstLaunch {
			m := newSetupModel(a.stores, a.cfg, a.width, a.height)
			a.setup = &m
			a.current = viewSetup
		} else {
			m := newDashboardModel(a.stores, a.sprint, a.width, a.height)
			a.dashboard = &m
			a.current = viewDashboard
		}
		return a, a.initActive()

	case errMsg:
		a.err = msg.err
		a.current = viewError
		return a, nil

	// Setup complete: sprint was created, move to dashboard.
	case setupDoneMsg:
		a.sprint = msg.sprint
		m := newDashboardModel(a.stores, a.sprint, a.width, a.height)
		a.dashboard = &m
		a.current = viewDashboard
		return a, a.initActive()

	// Sprint stats overlay closed: restore previous view.
	case closeStatsMsg:
		a.current = a.prev
		return a, nil

	case tea.KeyMsg:
		// Let the active sub-model handle the key first.
		model, cmd := a.delegateKey(msg)
		if cmd != nil || model.current != a.current {
			return model, cmd
		}

		// Global navigation (only when not in setup or a form).
		if a.current == viewSetup || a.current == viewLoading || a.current == viewError {
			return a, nil
		}

		switch {
		case key.Matches(msg, Global.GoalsBoard):
			return a.navigateGoalsBoard()

		case key.Matches(msg, Global.SprintStats):
			return a.navigateSprintStats()

		case key.Matches(msg, Global.Monthly):
			return a.navigateMonthly()

		case key.Matches(msg, Global.Back):
			if a.current == viewDashboard {
				return a, tea.Quit
			}
			a.current = viewDashboard
			return a, nil
		}

		return a, nil
	}

	// For all other messages, delegate to the active sub-model.
	return a.delegateMsg(msg)
}

func (a App) View() string {
	switch a.current {
	case viewLoading:
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(config.ColorMuted)).
			Render("loading lira...")

	case viewError:
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(config.ColorPriorityHigh)).
			Render(fmt.Sprintf("error: %v\n\nPress q to quit.", a.err))

	case viewSetup:
		if a.setup != nil {
			return a.setup.View()
		}
	case viewDashboard:
		if a.dashboard != nil {
			return a.dashboard.View()
		}
	case viewGoalsBoard:
		if a.goalsBoard != nil {
			return a.goalsBoard.View()
		}
	case viewSprintStats:
		if a.sprintStats != nil {
			return a.sprintStats.View()
		}
	case viewMonthly:
		if a.monthly != nil {
			return a.monthly.View()
		}
	}
	return ""
}

// --- navigation helpers ---

func (a App) navigateGoalsBoard() (App, tea.Cmd) {
	if a.current == viewGoalsBoard {
		a.current = viewDashboard
		return a, nil
	}
	if a.goalsBoard == nil {
		m := newGoalsBoardModel(a.stores, a.sprint, a.width, a.height)
		a.goalsBoard = &m
	}
	a.current = viewGoalsBoard
	return a, a.initActive()
}

func (a App) navigateSprintStats() (App, tea.Cmd) {
	if a.current == viewSprintStats {
		a.current = a.prev
		return a, nil
	}
	if a.sprintStats == nil {
		m := newSprintStatsModel(a.stores, a.sprint, a.width, a.height)
		a.sprintStats = &m
	}
	a.prev = a.current
	a.current = viewSprintStats
	return a, a.initActive()
}

func (a App) navigateMonthly() (App, tea.Cmd) {
	if a.current == viewMonthly {
		a.current = viewDashboard
		return a, nil
	}
	if a.monthly == nil {
		m := newMonthlyModel(a.stores, a.width, a.height)
		a.monthly = &m
	}
	a.current = viewMonthly
	return a, a.initActive()
}

// --- delegation helpers ---

// delegateKey routes a key message to the active sub-model, returning the
// updated App and any command. Returns (a, nil) if no sub-model is active.
func (a App) delegateKey(msg tea.KeyMsg) (App, tea.Cmd) {
	return a.delegateMsg(msg)
}

// delegateMsg routes any message to the active sub-model.
func (a App) delegateMsg(msg tea.Msg) (App, tea.Cmd) {
	var cmd tea.Cmd
	switch a.current {
	case viewSetup:
		if a.setup != nil {
			*a.setup, cmd = a.setup.Update(msg)
		}
	case viewDashboard:
		if a.dashboard != nil {
			*a.dashboard, cmd = a.dashboard.Update(msg)
		}
	case viewGoalsBoard:
		if a.goalsBoard != nil {
			*a.goalsBoard, cmd = a.goalsBoard.Update(msg)
		}
	case viewSprintStats:
		if a.sprintStats != nil {
			*a.sprintStats, cmd = a.sprintStats.Update(msg)
		}
	case viewMonthly:
		if a.monthly != nil {
			*a.monthly, cmd = a.monthly.Update(msg)
		}
	}
	return a, cmd
}

// initActive calls Init() on whichever sub-model just became active.
func (a App) initActive() tea.Cmd {
	switch a.current {
	case viewSetup:
		if a.setup != nil {
			return a.setup.Init()
		}
	case viewDashboard:
		if a.dashboard != nil {
			return a.dashboard.Init()
		}
	case viewGoalsBoard:
		if a.goalsBoard != nil {
			return a.goalsBoard.Init()
		}
	case viewSprintStats:
		if a.sprintStats != nil {
			return a.sprintStats.Init()
		}
	case viewMonthly:
		if a.monthly != nil {
			return a.monthly.Init()
		}
	}
	return nil
}

// resizeActive forwards a WindowSizeMsg to whichever sub-model is active.
func (a App) resizeActive() tea.Cmd {
	msg := tea.WindowSizeMsg{Width: a.width, Height: a.height}
	_, cmd := a.delegateMsg(msg)
	return cmd
}

// --- startup command ---

// startupCmd is the Bubbletea command that runs the full startup sequence:
// load config → open DB → run migrations → advance sprint if expired.
func startupCmd() tea.Msg {
	cfg, err := config.Load()
	if err != nil {
		return errMsg{fmt.Errorf("load config: %w", err)}
	}

	db, err := liradb.Open(cfg.DBPath)
	if err != nil {
		return errMsg{fmt.Errorf("open db: %w", err)}
	}

	sprints := store.NewSprintStore(db)

	hasAny, err := sprints.HasAny()
	if err != nil {
		return errMsg{fmt.Errorf("check sprints: %w", err)}
	}

	// First launch: no sprint has ever been created.
	if !hasAny {
		return appReadyMsg{db: db, cfg: cfg, sprint: nil, isFirstLaunch: true}
	}

	// Advance sprint if one or more have expired since the last app launch.
	sprint, err := sprints.AdvanceIfExpired(cfg.SprintLengthDays)
	if err != nil {
		return errMsg{fmt.Errorf("advance sprint: %w", err)}
	}

	return appReadyMsg{db: db, cfg: cfg, sprint: sprint, isFirstLaunch: false}
}

// --- message types ---

type appReadyMsg struct {
	db            *sql.DB
	cfg           *config.Config
	sprint        *models.Sprint
	isFirstLaunch bool
}

type setupDoneMsg struct {
	sprint *models.Sprint
}

type closeStatsMsg struct{}

type errMsg struct {
	err error
}
