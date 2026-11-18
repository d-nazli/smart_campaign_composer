-- +goose Up

-- The campaign's short name for the list, written by the model with the
-- audience rule (at most 40 characters). Campaigns made before have none; the
-- list shows their brief instead.
ALTER TABLE campaigns ADD COLUMN name TEXT NOT NULL DEFAULT ''
    CONSTRAINT campaigns_name_check CHECK (char_length(name) <= 40 AND position(E'\n' IN name) = 0);

-- +goose Down
ALTER TABLE campaigns DROP COLUMN name;
