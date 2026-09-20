-- One row per (rumour_id, user_id) ever, active or not -- unliking
-- soft-deletes (sets deleted_at) rather than removing the row, so a
-- re-like un-deletes the same row via ON CONFLICT instead of inserting a
-- second one.
CREATE TABLE likes (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rumour_id   UUID NOT NULL REFERENCES rumours (id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ,
    UNIQUE (rumour_id, user_id)
);
CREATE INDEX idx_likes_rumour_id_active ON likes (rumour_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_likes_user_id_active ON likes (user_id) WHERE deleted_at IS NULL;
