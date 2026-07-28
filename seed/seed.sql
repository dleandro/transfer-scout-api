-- Local dev seed data. Players/rumours are not seeded — they are created by
-- the extraction + upsert pipeline (milestones 1.3/1.4).

INSERT INTO clubs (name, short_name) VALUES
    ('Arsenal', 'ARS'),
    ('Aston Villa', 'AVL'),
    ('Bournemouth', 'BOU'),
    ('Brentford', 'BRE'),
    ('Brighton & Hove Albion', 'BHA'),
    ('Burnley', 'BUR'),
    ('Chelsea', 'CHE'),
    ('Crystal Palace', 'CRY'),
    ('Everton', 'EVE'),
    ('Fulham', 'FUL'),
    ('Leeds United', 'LEE'),
    ('Liverpool', 'LIV'),
    ('Manchester City', 'MCI'),
    ('Manchester United', 'MUN'),
    ('Newcastle United', 'NEW'),
    ('Nottingham Forest', 'NFO'),
    ('Sunderland', 'SUN'),
    ('Tottenham Hotspur', 'TOT'),
    ('West Ham United', 'WHU'),
    ('Wolverhampton Wanderers', 'WOL')
ON CONFLICT DO NOTHING;

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
