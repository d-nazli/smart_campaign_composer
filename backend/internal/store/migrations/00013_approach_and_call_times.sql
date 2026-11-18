-- +goose Up

-- The approach a variant's source text takes (D105): one of a fixed list,
-- chosen by the model for the brief. Only source rows have one; a translation
-- carries its source's. NULL for texts written before, and for the built-in
-- template (it takes no deliberate approach).
ALTER TABLE campaign_contents ADD COLUMN approach TEXT
    CONSTRAINT campaign_contents_approach_check CHECK (approach IN ('aciliyet', 'duygusal', 'fayda', 'merak', 'kisisel')),
    ADD CONSTRAINT campaign_contents_approach_source_check CHECK (approach IS NULL OR source_version IS NULL);

-- When each provider call of a delivery started (retries included), so the
-- sending screen can show the busiest second of each channel (D107).
ALTER TABLE deliveries ADD COLUMN call_starts TIMESTAMPTZ[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE deliveries DROP COLUMN call_starts;
ALTER TABLE campaign_contents DROP CONSTRAINT campaign_contents_approach_source_check;
ALTER TABLE campaign_contents DROP COLUMN approach;
