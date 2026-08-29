-- Players are matched case-insensitively by name during extraction
-- clustering (see internal/cluster). Without a unique constraint,
-- get-or-create races/typo variants would silently create duplicate
-- player rows and fragment rumours that should cluster together.
--
-- Known limitation: two distinct real players who happen to share an
-- exact name would incorrectly merge. Acceptable for MVP; revisit with a
-- richer disambiguation key (e.g. birth year, current club) if it becomes
-- a real problem.
DROP INDEX IF EXISTS idx_players_name;
CREATE UNIQUE INDEX idx_players_name ON players (lower(name));
