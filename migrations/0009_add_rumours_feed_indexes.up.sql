-- GET /rumours sorts every row by updated_at, so the app's main screen is a
-- seq scan + sort that grows with the table.
CREATE INDEX idx_rumours_updated_at ON rumours (updated_at DESC);

-- The club filter matches either side of a deal: (to_club_id = $1 OR
-- from_club_id = $1). from_club_id had no index, and to_club_id was only the
-- non-leftmost column of UNIQUE(player_id, to_club_id, transfer_window),
-- which the planner cannot use on its own.
CREATE INDEX idx_rumours_to_club_id ON rumours (to_club_id);
CREATE INDEX idx_rumours_from_club_id ON rumours (from_club_id);
