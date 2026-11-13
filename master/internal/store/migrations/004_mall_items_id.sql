ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS mall_items_id bigint NOT NULL DEFAULT 0;
