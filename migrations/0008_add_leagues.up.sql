CREATE TABLE leagues (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    short_name  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_leagues_name ON leagues (lower(name));

-- Nullable on purpose: a rumour can mention a club from a league that
-- doesn't exist in this table yet (e.g. a player leaving Real Madrid for a
-- PL club) -- GetOrCreateClub creates that club at runtime with no known
-- league, and it must stay NULL rather than default to the Premier League.
ALTER TABLE clubs ADD COLUMN league_id UUID REFERENCES leagues (id) ON DELETE SET NULL;

INSERT INTO leagues (name, short_name) VALUES ('Premier League', 'PL');

-- Backfills league_id on any of the 20 PL clubs that already exist in this
-- database (the real upgrade path: a dev/prod DB that ran seed/seed.sql
-- before this migration existed). A freshly bootstrapped database has no
-- club rows yet at this point in the migrate-then-seed sequence, so this is
-- a no-op there -- GetOrCreateClub's own ON CONFLICT stamp (mirroring
-- crest_url) covers those clubs the next time each is referenced.
UPDATE clubs SET league_id = (SELECT id FROM leagues WHERE lower(name) = 'premier league')
WHERE lower(name) IN (
    'arsenal', 'aston villa', 'bournemouth', 'brentford', 'brighton & hove albion',
    'burnley', 'chelsea', 'crystal palace', 'everton', 'fulham', 'leeds united',
    'liverpool', 'manchester city', 'manchester united', 'newcastle united',
    'nottingham forest', 'sunderland', 'tottenham hotspur', 'west ham united',
    'wolverhampton wanderers'
);
