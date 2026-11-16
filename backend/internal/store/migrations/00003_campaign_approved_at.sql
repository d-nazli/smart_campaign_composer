-- +goose Up
ALTER TABLE campaigns ADD COLUMN approved_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE campaigns DROP COLUMN approved_at;
