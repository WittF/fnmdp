CREATE TABLE IF NOT EXISTS schema_migrations (
    version integer PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS projects (
    bili_project_id bigint PRIMARY KEY,
    name text NOT NULL DEFAULT '',
    flag integer NOT NULL DEFAULT 0,
    flag_text text NOT NULL DEFAULT '',
    hot boolean NOT NULL DEFAULT false,
    wish_count bigint NOT NULL DEFAULT 0,
    price_low integer NOT NULL DEFAULT 0,
    price_high integer NOT NULL DEFAULT 0,
    salable_low integer NOT NULL DEFAULT 0,
    salable_high integer NOT NULL DEFAULT 0,
    status integer NOT NULL DEFAULT 0,
    is_sale integer NOT NULL DEFAULT 0,
    sale_start bigint NOT NULL DEFAULT 0,
    sale_end bigint NOT NULL DEFAULT 0,
    count_down bigint NOT NULL DEFAULT 0,
    server_ts bigint NOT NULL DEFAULT 0,
    priority integer NOT NULL DEFAULT 0,
    target_interval_ms integer NOT NULL DEFAULT 0,
    replica_factor integer NOT NULL DEFAULT 0,
    enabled boolean NOT NULL DEFAULT true,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS screen_state (
    project_id bigint NOT NULL,
    screen_id bigint NOT NULL,
    name text NOT NULL DEFAULT '',
    start_time bigint NOT NULL DEFAULT 0,
    flag integer NOT NULL DEFAULT 0,
    flag_text text NOT NULL DEFAULT '',
    clickable boolean NOT NULL DEFAULT false,
    sale_start bigint NOT NULL DEFAULT 0,
    sale_end bigint NOT NULL DEFAULT 0,
    server_ts bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, screen_id)
);

CREATE TABLE IF NOT EXISTS sku_state (
    project_id bigint NOT NULL,
    screen_id bigint NOT NULL,
    sku_id bigint NOT NULL,
    flag integer NOT NULL DEFAULT 0,
    flag_text text NOT NULL DEFAULT '',
    clickable boolean NOT NULL DEFAULT false,
    price_fen integer NOT NULL DEFAULT 0,
    descr text NOT NULL DEFAULT '',
    server_ts bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, screen_id, sku_id)
);

CREATE TABLE IF NOT EXISTS state_events (
    id bigserial PRIMARY KEY,
    project_id bigint NOT NULL DEFAULT 0,
    screen_id bigint NOT NULL DEFAULT 0,
    sku_id bigint NOT NULL DEFAULT 0,
    event_type text NOT NULL,
    severity text NOT NULL,
    old_flag integer NOT NULL DEFAULT 0,
    new_flag integer NOT NULL DEFAULT 0,
    old_clickable boolean NOT NULL DEFAULT false,
    new_clickable boolean NOT NULL DEFAULT false,
    old_price_fen integer NOT NULL DEFAULT 0,
    price_fen integer NOT NULL DEFAULT 0,
    flag_text text NOT NULL DEFAULT '',
    server_ts bigint NOT NULL DEFAULT 0,
    node_id text NOT NULL DEFAULT '',
    details jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_state_events_project_created ON state_events (project_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_state_events_type ON state_events (event_type);
CREATE INDEX IF NOT EXISTS idx_state_events_severity ON state_events (severity);
