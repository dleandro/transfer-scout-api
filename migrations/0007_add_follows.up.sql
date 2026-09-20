-- One row per (user_id, club_id) ever, active or not -- unfollowing
-- soft-deletes (sets deleted_at) rather than removing the row, so a
-- re-follow un-deletes the same row via ON CONFLICT instead of inserting a
-- second one. Mirrors migrations/0006_add_likes.up.sql.
CREATE TABLE follows (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    club_id     UUID NOT NULL REFERENCES clubs (id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ,
    UNIQUE (user_id, club_id)
);
CREATE INDEX idx_follows_user_id_active ON follows (user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_follows_club_id_active ON follows (club_id) WHERE deleted_at IS NULL;
