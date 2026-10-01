CREATE TABLE IF NOT EXISTS accounts (
    uid BIGSERIAL PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    u_name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sessions (
    access_key TEXT PRIMARY KEY,
    uid BIGINT NOT NULL REFERENCES accounts(uid) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS sessions_uid_idx ON sessions (uid);