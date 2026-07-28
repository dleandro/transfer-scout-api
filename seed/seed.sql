-- Local dev seed data. Sources are seeded without feed_url; real RSS URLs
-- are populated in milestone 1.2. Players/rumours are not seeded — they are
-- created by the extraction + upsert pipeline (milestones 1.3/1.4).

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

INSERT INTO sources (name) VALUES
    ('Sky Sports'),
    ('BBC Sport'),
    ('The Athletic'),
    ('The Guardian Football'),
    ('Daily Mail Sport'),
    ('The Mirror Football'),
    ('talkSPORT'),
    ('Football Insider'),
    ('GiveMeSport'),
    ('Fabrizio Romano')
ON CONFLICT (name) DO NOTHING;
