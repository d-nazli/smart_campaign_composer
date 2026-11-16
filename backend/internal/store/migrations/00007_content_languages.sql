-- +goose Up

-- A campaign now has one text per language (tr, en, de). The primary key grows
-- a language part; existing rows are Turkish, which is the default and the
-- reference language the others must mirror.
ALTER TABLE campaign_contents
    ADD COLUMN language TEXT NOT NULL DEFAULT 'tr'
        CONSTRAINT campaign_contents_language_check CHECK (language IN ('tr', 'en', 'de'));
ALTER TABLE campaign_contents DROP CONSTRAINT campaign_contents_pkey;
ALTER TABLE campaign_contents ADD PRIMARY KEY (campaign_id, language);

-- Which language a model attempt was for (the audit trail has one row per attempt).
ALTER TABLE llm_generations ADD COLUMN language TEXT NOT NULL DEFAULT 'tr';

-- How many reachable people speak each language, e.g. {"tr": 120, "en": 45}.
-- Shown on the approval screen; counted again when the audience is built.
ALTER TABLE campaigns ADD COLUMN language_counts JSONB;

-- +goose Down
ALTER TABLE campaigns DROP COLUMN language_counts;
ALTER TABLE llm_generations DROP COLUMN language;
DELETE FROM campaign_contents WHERE language <> 'tr';
ALTER TABLE campaign_contents DROP CONSTRAINT campaign_contents_pkey;
ALTER TABLE campaign_contents ADD PRIMARY KEY (campaign_id);
ALTER TABLE campaign_contents DROP COLUMN language;
