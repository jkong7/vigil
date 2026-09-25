CREATE TABLE IF NOT EXISTS check_results (
    id          BIGSERIAL PRIMARY KEY,
    monitor     TEXT        NOT NULL,
    up          BOOLEAN     NOT NULL,
    latency_ms  DOUBLE PRECISION NOT NULL,
    status_code INTEGER,
    error       TEXT,
    checked_at  TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS check_results_monitor_time ON check_results (monitor, checked_at DESC);

CREATE TABLE IF NOT EXISTS incidents (
    id          BIGSERIAL PRIMARY KEY,
    monitor     TEXT        NOT NULL,
    cause       TEXT        NOT NULL,
    started_at  TIMESTAMPTZ NOT NULL,
    resolved_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS incidents_one_open ON incidents (monitor) WHERE resolved_at IS NULL;
