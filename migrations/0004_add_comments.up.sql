-- The CHECK is defense-in-depth; primary body-length validation happens
-- in the handler so a violation returns a clean 400 rather than a raw
-- constraint-violation error.
CREATE TABLE comments (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rumour_id   UUID NOT NULL REFERENCES rumours (id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    body        TEXT NOT NULL CHECK (char_length(body) BETWEEN 1 AND 2000),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_comments_rumour_id ON comments (rumour_id, created_at);
