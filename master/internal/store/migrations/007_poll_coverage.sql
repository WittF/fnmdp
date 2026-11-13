CREATE TABLE IF NOT EXISTS poll_coverage (
    project_id bigint NOT NULL,
    start_ts   bigint NOT NULL,
    end_ts     bigint NOT NULL,
    PRIMARY KEY (project_id, start_ts)
);

CREATE INDEX IF NOT EXISTS idx_poll_coverage_project_end
    ON poll_coverage (project_id, end_ts DESC);
