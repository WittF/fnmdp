ALTER TABLE state_events
    ADD COLUMN IF NOT EXISTS old_price_fen integer NOT NULL DEFAULT 0;
