-- +goose Up
-- How often a delivery went back to the queue, and how many of those were 429s.
-- Kept in the row (not in process memory) so the numbers survive restarts and
-- the api can report them without talking to the worker.
ALTER TABLE deliveries
    ADD COLUMN retries      INT NOT NULL DEFAULT 0,
    ADD COLUMN rate_limited INT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE deliveries DROP COLUMN rate_limited, DROP COLUMN retries;
