CREATE TABLE IF NOT EXISTS devices (
    id UUID PRIMARY KEY,
    token_hash BYTEA NOT NULL UNIQUE,
    platform TEXT NOT NULL CHECK (platform IN ('ios', 'android', 'macos')),
    display_name TEXT NOT NULL DEFAULT '',
    push_token TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS subscriptions (
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    symbol TEXT NOT NULL,
    PRIMARY KEY (device_id, symbol)
);

CREATE TABLE IF NOT EXISTS ios_rule_snapshots (
    device_id UUID PRIMARY KEY REFERENCES devices(id) ON DELETE CASCADE,
    version BIGINT NOT NULL,
    payload JSONB NOT NULL,
    foreground_lease_until TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ios_events (
    id TEXT PRIMARY KEY,
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    acknowledged_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS ios_events_device_created_idx
    ON ios_events (device_id, created_at DESC);
