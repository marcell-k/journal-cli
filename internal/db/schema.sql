CREATE TABLE IF NOT EXISTS projects (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    name    TEXT NOT NULL UNIQUE
);

-- seed a default so the list is never empty --
INSERT OR IGNORE INTO projects (name) VALUES ('work');

CREATE TABLE IF NOT EXISTS weekly_goals (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    week_start  DATE NOT NULL,
    day         TEXT NOT NULL,
    goal        TEXT NOT NULL,
    done        BOOLEAN NOT NULL DEFAULT 0,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_weekly_goals_week_start ON weekly_goals(week_start);

CREATE TABLE IF NOT EXISTS blocks (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    date            DATE NOT NULL,
    block_num       INTEGER NOT NULL,
    day             TEXT NOT NULL,
    project_id      INTEGER REFERENCES projects(id),

    outcome         TEXT,
    context_reload  TEXT,
    first_action    TEXT,

    deliverable     TEXT,
    done_notes      TEXT,
    not_done_notes  TEXT,
    next_step       TEXT,
    files_links     TEXT,

    focus_quality   INTEGER CHECK (focus_quality BETWEEN 1 AND 5),
    tweak           TEXT,

    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    closed_at       TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_blocks_date ON blocks(date);
CREATE UNIQUE INDEX IF NOT EXISTS idx_blocks_date_num ON blocks(date, block_num);

CREATE TABLE IF NOT EXISTS daily_checkin (
    date          DATE PRIMARY KEY,
    sleep_hours   REAL,
    sleep_quality INTEGER CHECK (sleep_quality BETWEEN 1 AND 10),
    feel          INTEGER CHECK (feel BETWEEN 1 AND 10),
    notes         TEXT,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
