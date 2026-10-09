-- Run results schema, ADR 0002: normalized runs/jobs/steps mirroring the
-- event envelope's three levels, plus a JSON payload column reserved on
-- steps for future sub-step data (e.g. per-test outcomes).

-- +goose Up
CREATE TABLE runs (
    id INTEGER PRIMARY KEY,
    run_id TEXT NOT NULL UNIQUE,
    pipeline TEXT NOT NULL,
    status TEXT NOT NULL,
    started_at TEXT NOT NULL,
    finished_at TEXT
);

CREATE TABLE jobs (
    id INTEGER PRIMARY KEY,
    run_id INTEGER NOT NULL REFERENCES runs(id),
    name TEXT NOT NULL,
    stage TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    started_at TEXT NOT NULL,
    finished_at TEXT,
    UNIQUE (run_id, name)
);

CREATE TABLE steps (
    id INTEGER PRIMARY KEY,
    job_id INTEGER NOT NULL REFERENCES jobs(id),
    name TEXT NOT NULL,
    status TEXT NOT NULL,
    exit_code INTEGER,
    duration_ms INTEGER,
    payload TEXT NOT NULL DEFAULT '{}',
    UNIQUE (job_id, name)
);

-- +goose Down
DROP TABLE steps;
DROP TABLE jobs;
DROP TABLE runs;
