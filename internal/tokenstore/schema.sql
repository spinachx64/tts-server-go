-- Managed by the external token issuer. expires_at is a UTC Unix timestamp in seconds.
CREATE TABLE IF NOT EXISTS tokens (
    token_hash TEXT PRIMARY KEY NOT NULL CHECK (length(token_hash) = 64),
    user_id TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    expires_at INTEGER,
    created_at INTEGER NOT NULL DEFAULT (unixepoch())
);
