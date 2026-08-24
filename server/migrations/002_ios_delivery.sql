ALTER TABLE devices
    ADD COLUMN IF NOT EXISTS push_environment TEXT NOT NULL DEFAULT 'sandbox'
        CHECK (push_environment IN ('sandbox', 'production'));

ALTER TABLE ios_events
    ADD COLUMN IF NOT EXISTS last_delivery_attempt_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS delivery_attempts INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS delivered_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS ios_live_activities (
    activity_id TEXT PRIMARY KEY,
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    push_token TEXT NOT NULL,
    symbol TEXT NOT NULL,
    environment TEXT NOT NULL CHECK (environment IN ('sandbox', 'production')),
    expires_at TIMESTAMPTZ NOT NULL,
    last_update_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS ios_live_activities_symbol_idx
    ON ios_live_activities (symbol, expires_at);
