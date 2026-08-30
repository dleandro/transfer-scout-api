-- Users authenticate via Google OAuth only (see internal/auth). email is
-- deliberately NOT unique -- google_sub (Google's durable identity key) is
-- the real identity; email isn't guaranteed permanently stable.
CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    google_sub    TEXT NOT NULL UNIQUE,
    email         TEXT NOT NULL,
    display_name  TEXT NOT NULL,
    avatar_url    TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_users_email ON users (lower(email));
