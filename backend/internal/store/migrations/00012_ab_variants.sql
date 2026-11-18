-- +goose Up

-- A/B variants (D102). A campaign has Variant A and, if the marketer adds it,
-- Variant B: two different texts with the same offer. Each variant has its own
-- source text (tr-TR) and its own translations, so the staleness of a
-- translation is counted against its own variant's source row.
ALTER TABLE campaign_contents ADD COLUMN variant TEXT NOT NULL DEFAULT 'A'
    CONSTRAINT campaign_contents_variant_check CHECK (variant IN ('A', 'B'));
ALTER TABLE campaign_contents DROP CONSTRAINT campaign_contents_pkey;
ALTER TABLE campaign_contents ADD PRIMARY KEY (campaign_id, variant, locale);

-- Which variant the model wrote (every attempt is audited, B too).
ALTER TABLE llm_generations ADD COLUMN variant TEXT NOT NULL DEFAULT 'A'
    CONSTRAINT llm_generations_variant_check CHECK (variant IN ('A', 'B'));

-- The variant a delivery carries, decided when it is queued from a hash of
-- (campaign, person): the person's locale plays no part, and both channels of
-- one person get the same variant. UNIQUE(campaign_id, user_id, channel) still
-- holds, so a person gets one variant, once.
ALTER TABLE deliveries ADD COLUMN variant TEXT NOT NULL DEFAULT 'A'
    CONSTRAINT deliveries_variant_check CHECK (variant IN ('A', 'B'));
CREATE INDEX deliveries_campaign_variant_idx ON deliveries (campaign_id, variant, channel, status);

-- How many people each variant reaches, e.g.
-- {"A": {"people": 1004, "sms": 812, "email": 905}, "B": {...}}. Counted when
-- Variant B is written and again at approval; NULL without Variant B.
ALTER TABLE campaigns ADD COLUMN variant_counts JSONB;

-- +goose Down
ALTER TABLE campaigns DROP COLUMN variant_counts;

DROP INDEX deliveries_campaign_variant_idx;
ALTER TABLE deliveries DROP COLUMN variant;

ALTER TABLE llm_generations DROP COLUMN variant;

DELETE FROM campaign_contents WHERE variant = 'B';
ALTER TABLE campaign_contents DROP CONSTRAINT campaign_contents_pkey;
ALTER TABLE campaign_contents ADD PRIMARY KEY (campaign_id, locale);
ALTER TABLE campaign_contents DROP COLUMN variant;
