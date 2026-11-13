-- Speeds up per-SKU status-timeline history queries (/api/history).
CREATE INDEX IF NOT EXISTS idx_state_events_sku_created
    ON state_events (project_id, sku_id, created_at DESC);
