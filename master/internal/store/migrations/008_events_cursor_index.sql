CREATE INDEX IF NOT EXISTS idx_state_events_created_id
    ON state_events (created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_state_events_project_created_id
    ON state_events (project_id, created_at DESC, id DESC);
