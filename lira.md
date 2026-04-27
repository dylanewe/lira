# Lira
- Jira-style Task Management for Life
- Build habits, track goals, run sprints

---

## Product Vision

A terminal UI (TUI) personal productivity tool modeled after Jira. Designed for tracking real-life tasks: chores, fitness, reading, habits, etc. Single-user, fully local.

---

## Stack

| Layer     | Technology          |
|-----------|---------------------|
| Backend   | Go                  |
| Database  | SQLite (local file) |
| UI        | Bubbletea / Charm   |
| Container | Docker (optional)   |

---

## Data Model

### Hierarchy
```
Goal (Epic)
└── Step (Story)
    └── Task (Action Item)

Task (standalone, no Step or Goal required)
```

### Goal
- Title (required)
- Description (plain text, optional)
- Priority: Low | Medium | High
- Status: Todo | In Progress | Done
- Color (from Horizon palette, user picks or auto-assigned)
- ID: `L-XXXX` (auto-incremented from 0000)

### Step
- Title (required)
- Description (plain text, optional)
- Priority: Low | Medium | High
- Status: Todo | In Progress | Done
- Must be linked to a Goal
- Inherits color from linked Goal
- ID: `L-XXXX`

### Task
- Title (required)
- Description (plain text, optional)
- Priority: Low | Medium | High
- Status: Todo | In Progress | Done
- Can be standalone OR linked to a Step (and by extension, its Goal)
- If linked to a Step, inherits color from that Step's Goal
- Repeatable flag: if set, task is automatically re-created in the next sprint
- ID: `L-XXXX`

### ID Format
- All tickets (Goals, Steps, Tasks) share a single global counter
- Format: `L-XXXX` starting from `L-0000`
- IDs are never reused after deletion

---

## Sprints

- **Sprint length**: configurable by the user (default: 7 days)
- **First sprint**: started by the user during the first-launch setup screen
- **Subsequent sprints**: automatically close and start on app launch if the current sprint's end time has passed
- **Auto-close trigger**: sprint boundaries are evaluated at launch time only — no background daemon
- **Naming**: Sprint 1, Sprint 2, Sprint 3 ... (no custom names)
- **Carry-over**: any incomplete (non-Done) Steps and Tasks at sprint end are automatically moved to the next sprint
- **Repeatable Tasks**: re-created fresh each sprint (status resets to Todo), even if completed in the prior sprint
- Goals are **not** sprint-scoped — they persist across sprints until marked Done

---

## Views & Navigation

### 1. Main Dashboard (default view)
Kanban board showing Steps and Tasks for the current sprint.

```
┌──────────────┬──────────────────┬──────────────┐
│     Todo     │   In Progress    │     Done     │
├──────────────┼──────────────────┼──────────────┤
│ L-0002 Step  │ L-0001 Step      │ L-0005 Step  │
│   L-0003 ↳Task│                │   L-0006 ↳Task│
│   L-0004 ↳Task│                │              │
│ L-0007 Task  │                  │              │
└──────────────┴──────────────────┴──────────────┘
```

- Tasks that are children of a Step are shown directly below their Step with a `↳` prefix
- Standalone Tasks appear as top-level items
- Items are color-coded based on their linked Goal's color (or default color if standalone)

### 2. Goals Board — `G` (case-insensitive)
- Separate full-screen kanban board for Goals only
- Same Todo / In Progress / Done column layout
- When a Goal is moved to Done:
  - All linked Steps and Tasks are automatically set to Done regardless of current status
- Navigation and controls mirror the main dashboard

### 3. Sprint Stats — `Y` (case-insensitive)
- Popup/overlay showing stats for a sprint
- Navigate between sprints with `←` / `→` arrow keys
- Stats per sprint:
  - Total tickets created
  - Tickets completed (moved to Done)
  - Tickets carried over
  - Sprint velocity (tickets completed per day)
  - Repeatable tasks completed vs. total

### 4. Monthly Analysis — `M` (case-insensitive)
- Separate full-screen view
- Covers the current calendar month
- Metrics:
  - Total tickets completed
  - Sprint velocity trend (per sprint within the month)
  - Habit streaks (consecutive sprints where **all** repeatable tasks were completed)

---

## Keyboard Controls

### Global
| Key              | Action                        |
|------------------|-------------------------------|
| `G` / `g`        | Toggle Goals Board            |
| `Y` / `y`        | Toggle Sprint Stats overlay   |
| `M` / `m`        | Toggle Monthly Analysis view  |
| `q`              | Quit / go back                |
| `?`              | Show keybindings help         |

### Navigation (Dashboard & Goals Board)
| Key                        | Action                          |
|----------------------------|---------------------------------|
| `←` `→` `↑` `↓`           | Move cursor between items       |
| `h` `l` `k` `j`           | Vim-style: left, right, up, down|
| `Space`                    | Select / deselect a ticket      |
| `Space` + `←`/`→` or `h`/`l` | Move selected ticket between columns |

### Ticket Management
| Key    | Action                          |
|--------|---------------------------------|
| `+`    | Open create ticket form         |
| `-`    | Delete selected ticket (with confirmation prompt) |
| `Enter`| Open / expand selected ticket   |

---

## Ticket Creation Form (`+`)

Multi-step form flow:

1. **Select type**: Goal / Step / Task
2. **Select priority**: Low / Medium / High
3. **Write title** (required)
4. **Write description** (optional, plain text)
5. **If Step**: select linked Goal from list
6. **If Task**: optionally link to a Step or Goal; toggle Repeatable flag
7. **If Goal**: select color from Horizon palette (or auto-assign next available)
8. **Confirm**: `Enter` to create — validates required fields before saving

---

## Ticket Deletion (`-`)

- Prompts: `Delete L-XXXX? [y/N]` (case-insensitive, default No)
- **If Goal deleted**: all linked Steps and Tasks are deleted, **unless** they are in the Done column
- **If Step deleted**: all linked Tasks are deleted, **unless** they are in the Done column
- **If Task deleted**: only that task is deleted

---

## Color System

- Color palette follows the **Horizon** theme
- All color constants are defined in `config/colors.go` for future configurability
- Colors are assigned to Goals; Steps and Tasks inherit the color of their linked Goal
- Standalone Steps/Tasks use a default neutral color
- During Goal creation, user can pick a color from the palette or leave it for auto-assignment (next unused color in rotation)

---

## Configuration

Stored in a local config file (e.g., `~/.lira/config.json` or similar):

| Setting         | Default  | Description                        |
|-----------------|----------|------------------------------------|
| `sprint_length` | 7 days   | Duration of each sprint in days    |
| `db_path`       | `~/.lira/lira.db` | Path to the SQLite database |

---

## File Structure

```
lira/
├── main.go
├── go.mod
├── go.sum
├── config/
│   ├── config.go          # Load/save config (sprint length, db path, etc.)
│   └── colors.go          # Horizon color palette constants
├── db/
│   ├── db.go              # SQLite connection and initialization
│   └── migrations/
│       └── 001_init.sql   # Schema: goals, steps, tasks, sprints tables
├── models/
│   ├── goal.go
│   ├── step.go
│   ├── task.go
│   └── sprint.go
├── store/
│   ├── goal_store.go      # CRUD for goals
│   ├── step_store.go      # CRUD for steps
│   ├── task_store.go      # CRUD for tasks
│   └── sprint_store.go    # CRUD + carry-over logic for sprints
├── tui/
│   ├── app.go             # Root Bubbletea model, view routing
│   ├── dashboard.go       # Main kanban board (Steps + Tasks)
│   ├── goals_board.go     # Goals kanban board
│   ├── sprint_stats.go    # Sprint stats overlay
│   ├── monthly.go         # Monthly analysis screen
│   ├── keybindings.go     # Shared key map definitions
│   └── forms/
│       ├── create.go      # Multi-step ticket creation form
│       └── confirm.go     # Delete confirmation prompt
└── ~/.lira/
    ├── lira.db            # SQLite database (runtime)
    └── config.json        # User config (runtime)
```

---

## First Launch Experience

When no database or sprint exists yet, the app shows a **setup screen** instead of the main dashboard:

1. Welcome message explaining Lira
2. **Configure sprint length** (default shown as 7 days, user can change)
3. **Create initial Goals** — user can add one or more Goals (title, priority, color)
4. **Create initial Steps/Tasks** — optional, user can add items linked to those Goals or standalone
5. **Start Sprint 1** — confirm button launches the first sprint and enters the main dashboard

The setup screen is a guided linear flow (not a free-form board). User can skip Steps/Tasks creation and go straight to starting the sprint — those can be added from the dashboard at any time.

---

## Out of Scope (MVP)

- Search and filtering
- Multi-user / profiles
- Due dates on individual tickets
- Markdown rendering in descriptions
- Custom sprint names
- Remote sync or export
