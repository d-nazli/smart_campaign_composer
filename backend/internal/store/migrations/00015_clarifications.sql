-- +goose Up

-- The questions the model asked about the brief and the person's answers, oldest
-- first: [{"question": "...", "answer": "...", "answered_at": "..."}]. An answer
-- resolves the audience again on the same campaign (no new campaign per answer).
ALTER TABLE campaigns ADD COLUMN clarifications JSONB NOT NULL DEFAULT '[]'::jsonb;

-- +goose Down
ALTER TABLE campaigns DROP COLUMN clarifications;
