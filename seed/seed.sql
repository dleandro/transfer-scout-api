-- Local dev seed data. Players/rumours are not seeded — they are created by
-- the extraction + upsert pipeline (milestones 1.3/1.4).
--
-- The club INSERT below is GENERATED from premierLeagueClubs in
-- internal/store/pl_clubs.go. Do not hand-edit it: add or remove clubs there
-- and run `go test ./internal/store/`, which prints the replacement block.
-- Seeding is an upsert, so re-running it backfills crest_url and league_id
-- onto clubs that were seeded before those columns existed.

INSERT INTO clubs (name, short_name, crest_url, league_id)
SELECT c.name, c.short_name, c.crest_url, l.id
FROM (VALUES
    ('Arsenal', 'ARS', 'https://upload.wikimedia.org/wikipedia/en/5/53/Arsenal_FC.svg'),
    ('Aston Villa', 'AVL', 'https://upload.wikimedia.org/wikipedia/en/9/9a/Aston_Villa_FC_new_crest.svg'),
    ('Bournemouth', 'BOU', 'https://upload.wikimedia.org/wikipedia/en/e/e5/AFC_Bournemouth_%282013%29.svg'),
    ('Brentford', 'BRE', 'https://upload.wikimedia.org/wikipedia/en/2/2a/Brentford_FC_crest.svg'),
    ('Brighton & Hove Albion', 'BHA', 'https://upload.wikimedia.org/wikipedia/en/d/d0/Brighton_and_Hove_Albion_FC_crest.svg'),
    ('Burnley', 'BUR', 'https://upload.wikimedia.org/wikipedia/en/6/6d/Burnley_FC_Logo.svg'),
    ('Chelsea', 'CHE', 'https://upload.wikimedia.org/wikipedia/en/c/cc/Chelsea_FC.svg'),
    ('Crystal Palace', 'CRY', 'https://upload.wikimedia.org/wikipedia/en/a/a2/Crystal_Palace_FC_logo_%282022%29.svg'),
    ('Everton', 'EVE', 'https://upload.wikimedia.org/wikipedia/en/7/7c/Everton_FC_logo.svg'),
    ('Fulham', 'FUL', 'https://upload.wikimedia.org/wikipedia/en/e/eb/Fulham_FC_%28shield%29.svg'),
    ('Leeds United', 'LEE', 'https://upload.wikimedia.org/wikipedia/en/5/54/Leeds_United_F.C._logo.svg'),
    ('Liverpool', 'LIV', 'https://upload.wikimedia.org/wikipedia/en/0/0c/Liverpool_FC.svg'),
    ('Manchester City', 'MCI', 'https://upload.wikimedia.org/wikipedia/en/e/eb/Manchester_City_FC_badge.svg'),
    ('Manchester United', 'MUN', 'https://upload.wikimedia.org/wikipedia/en/7/7a/Manchester_United_FC_crest.svg'),
    ('Newcastle United', 'NEW', 'https://upload.wikimedia.org/wikipedia/en/5/56/Newcastle_United_Logo.svg'),
    ('Nottingham Forest', 'NFO', 'https://upload.wikimedia.org/wikipedia/en/e/e5/Nottingham_Forest_F.C._logo.svg'),
    ('Sunderland', 'SUN', 'https://upload.wikimedia.org/wikipedia/en/7/77/Logo_Sunderland.svg'),
    ('Tottenham Hotspur', 'TOT', 'https://upload.wikimedia.org/wikipedia/en/b/b4/Tottenham_Hotspur.svg'),
    ('West Ham United', 'WHU', 'https://upload.wikimedia.org/wikipedia/en/c/c2/West_Ham_United_FC_logo.svg'),
    ('Wolverhampton Wanderers', 'WOL', 'https://upload.wikimedia.org/wikipedia/en/f/fc/Wolverhampton_Wanderers.svg')
) AS c(name, short_name, crest_url)
LEFT JOIN leagues l ON lower(l.name) = 'premier league'
ON CONFLICT (lower(name)) DO UPDATE
SET short_name = EXCLUDED.short_name,
    crest_url = COALESCE(EXCLUDED.crest_url, clubs.crest_url),
    league_id = COALESCE(EXCLUDED.league_id, clubs.league_id);

-- Real RSS feed URLs, verified reachable and well-formed as of 2026-07-28.
-- talkSPORT and The Athletic have no public RSS feed (both now serve a JS
-- SPA shell at every guessed /feed path; The Athletic is also paywalled) —
-- left NULL until/unless one turns up. Fabrizio Romano has no standalone
-- feed of his own; CaughtOffside republishes his exclusives and is used as
-- a proxy source for his reporting.
INSERT INTO sources (name, feed_url) VALUES
    ('Sky Sports', 'https://www.skysports.com/rss/12040'),
    ('BBC Sport', 'https://feeds.bbci.co.uk/sport/football/rss.xml'),
    ('The Athletic', NULL),
    ('The Guardian Football', 'https://www.theguardian.com/football/rss'),
    ('Daily Mail Sport', 'https://www.dailymail.co.uk/sport/index.rss'),
    ('The Mirror Football', 'https://www.mirror.co.uk/sport/football/rss.xml'),
    ('talkSPORT', NULL),
    ('Football Insider', 'https://www.footballinsider247.com/feed/'),
    ('GiveMeSport', 'https://www.givemesport.com/feed/'),
    ('Fabrizio Romano', 'https://www.caughtoffside.com/feed/')
ON CONFLICT (name) DO UPDATE SET feed_url = EXCLUDED.feed_url;
