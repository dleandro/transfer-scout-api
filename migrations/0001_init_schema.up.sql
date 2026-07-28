CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TYPE rumour_status AS ENUM (
    'rumoured',
    'talks',
    'advanced',
    'medical',
    'confirmed',
    'collapsed'
);

CREATE TABLE clubs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    short_name  TEXT,
    crest_url   TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_clubs_name ON clubs (lower(name));

CREATE TABLE players (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name             TEXT NOT NULL,
    current_club_id  UUID REFERENCES clubs (id),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_players_name ON players (lower(name));

CREATE TABLE sources (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name               TEXT NOT NULL UNIQUE,
    feed_url           TEXT,
    reliability_score  NUMERIC(5, 2) NOT NULL DEFAULT 50.00,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE articles (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id     UUID NOT NULL REFERENCES sources (id),
    url           TEXT NOT NULL UNIQUE,
    title         TEXT NOT NULL,
    content       TEXT,
    published_at  TIMESTAMPTZ,
    fetched_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed     BOOLEAN NOT NULL DEFAULT false,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_articles_unprocessed ON articles (fetched_at) WHERE processed = false;
CREATE INDEX idx_articles_source_id ON articles (source_id);

CREATE TABLE rumours (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    player_id        UUID NOT NULL REFERENCES players (id),
    from_club_id     UUID REFERENCES clubs (id),
    to_club_id       UUID NOT NULL REFERENCES clubs (id),
    transfer_window  TEXT NOT NULL,
    status           rumour_status NOT NULL DEFAULT 'rumoured',
    fee_min_eur      NUMERIC(12, 2),
    fee_max_eur      NUMERIC(12, 2),
    summary          TEXT,
    confidence       NUMERIC(4, 3),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (player_id, to_club_id, transfer_window)
);

CREATE INDEX idx_rumours_status ON rumours (status);
CREATE INDEX idx_rumours_transfer_window ON rumours (transfer_window);

CREATE TABLE rumour_events (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rumour_id   UUID NOT NULL REFERENCES rumours (id) ON DELETE CASCADE,
    article_id  UUID NOT NULL REFERENCES articles (id),
    source_id   UUID NOT NULL REFERENCES sources (id),
    status      rumour_status NOT NULL,
    fee_min_eur NUMERIC(12, 2),
    fee_max_eur NUMERIC(12, 2),
    summary     TEXT,
    confidence  NUMERIC(4, 3),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_rumour_events_rumour_id ON rumour_events (rumour_id);
CREATE UNIQUE INDEX idx_rumour_events_rumour_article ON rumour_events (rumour_id, article_id);
