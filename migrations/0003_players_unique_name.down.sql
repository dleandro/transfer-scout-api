DROP INDEX IF EXISTS idx_players_name;
CREATE INDEX idx_players_name ON players (lower(name));
