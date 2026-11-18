-- +goose Up

-- Translations are no longer edited by hand (D97): only the source text is.
-- A translation a person had edited becomes an ordinary translation that is
-- not up to date (source_version 0 is older than any source version), so it
-- is translated again before the campaign can be approved.
UPDATE campaign_contents
SET source = 'translated', source_version = 0, updated_at = now()
WHERE source = 'edited' AND source_version IS NOT NULL;

-- The database keeps it that way: the source row (no source_version) is
-- written by the model, the built-in template or a person; a translation row
-- (with a source_version) only by the translator.
ALTER TABLE campaign_contents ADD CONSTRAINT campaign_contents_translation_source_check CHECK (
    (source_version IS NULL AND source IN ('llm', 'fallback', 'edited'))
    OR (source_version IS NOT NULL AND source = 'translated'));

-- +goose Down
ALTER TABLE campaign_contents DROP CONSTRAINT campaign_contents_translation_source_check;
