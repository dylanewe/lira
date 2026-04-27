PRAGMA foreign_keys = ON;

-- User configuration (sprint length, etc.)
CREATE TABLE IF NOT EXISTS config (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- Global ticket ID counter (single row, always id=1)
CREATE TABLE IF NOT EXISTS ticket_counter (
    id       INTEGER PRIMARY KEY CHECK (id = 1),
    next_val INTEGER NOT NULL DEFAULT 0
);
INSERT INTO ticket_counter (id, next_val) VALUES (1, 0);

-- Sprints
CREATE TABLE IF NOT EXISTS sprints (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    number     INTEGER NOT NULL UNIQUE,
    start_date TEXT    NOT NULL,  -- ISO 8601: "2026-04-12T00:00:00Z"
    end_date   TEXT    NOT NULL,  -- ISO 8601
    status     TEXT    NOT NULL DEFAULT 'active'
                       CHECK (status IN ('active', 'closed'))
);

-- Goals (not sprint-scoped; persist until Done)
CREATE TABLE IF NOT EXISTS goals (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    ticket_id   TEXT    NOT NULL UNIQUE,  -- "L-0000"
    title       TEXT    NOT NULL,
    description TEXT    NOT NULL DEFAULT '',
    priority    TEXT    NOT NULL DEFAULT 'medium'
                        CHECK (priority IN ('low', 'medium', 'high')),
    status      TEXT    NOT NULL DEFAULT 'todo'
                        CHECK (status IN ('todo', 'in_progress', 'done')),
    color       TEXT    NOT NULL,         -- Horizon palette key, e.g. "primary"
    position    INTEGER NOT NULL DEFAULT 0,  -- display order within status column
    created_at  TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- Steps (must belong to a goal; are sprint-scoped for carry-over tracking)
CREATE TABLE IF NOT EXISTS steps (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    ticket_id   TEXT    NOT NULL UNIQUE,
    title       TEXT    NOT NULL,
    description TEXT    NOT NULL DEFAULT '',
    priority    TEXT    NOT NULL DEFAULT 'medium'
                        CHECK (priority IN ('low', 'medium', 'high')),
    status      TEXT    NOT NULL DEFAULT 'todo'
                        CHECK (status IN ('todo', 'in_progress', 'done')),
    goal_id     INTEGER REFERENCES goals(id),             -- nullable: done steps can outlive their goal
    sprint_id   INTEGER NOT NULL REFERENCES sprints(id),
    position    INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- Tasks (standalone or linked to a step/goal; optionally repeatable)
CREATE TABLE IF NOT EXISTS tasks (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    ticket_id   TEXT    NOT NULL UNIQUE,
    title       TEXT    NOT NULL,
    description TEXT    NOT NULL DEFAULT '',
    priority    TEXT    NOT NULL DEFAULT 'medium'
                        CHECK (priority IN ('low', 'medium', 'high')),
    status      TEXT    NOT NULL DEFAULT 'todo'
                        CHECK (status IN ('todo', 'in_progress', 'done')),
    step_id     INTEGER REFERENCES steps(id),   -- nullable; no CASCADE (handled in store)
    goal_id     INTEGER REFERENCES goals(id),   -- nullable; no CASCADE (handled in store)
    sprint_id   INTEGER NOT NULL REFERENCES sprints(id),
    repeatable  INTEGER NOT NULL DEFAULT 0 CHECK (repeatable IN (0, 1)),  -- 0=false, 1=true
    position    INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- Indexes for common lookups
CREATE INDEX IF NOT EXISTS idx_steps_goal_id   ON steps (goal_id);
CREATE INDEX IF NOT EXISTS idx_steps_sprint_id ON steps (sprint_id);
CREATE INDEX IF NOT EXISTS idx_tasks_step_id   ON tasks (step_id);
CREATE INDEX IF NOT EXISTS idx_tasks_goal_id   ON tasks (goal_id);
CREATE INDEX IF NOT EXISTS idx_tasks_sprint_id ON tasks (sprint_id);
