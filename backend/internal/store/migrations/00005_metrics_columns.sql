-- +goose Up
-- Provider response time: how many calls were made for a delivery and how long
-- they took in total (microseconds). Averages and sums are derived at scrape
-- time, so they survive restarts and need no worker-to-api channel.
ALTER TABLE deliveries
    ADD COLUMN provider_calls  INT    NOT NULL DEFAULT 0,
    ADD COLUMN provider_micros BIGINT NOT NULL DEFAULT 0;

-- Two facts about an LLM attempt that used to be implicit:
--   refused:   the model declined to answer (the run stopped at this attempt)
--   fell_back: the run ended with the built-in template (set on its last attempt)
ALTER TABLE llm_generations
    ADD COLUMN refused   BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN fell_back BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE llm_generations DROP COLUMN fell_back, DROP COLUMN refused;
ALTER TABLE deliveries DROP COLUMN provider_micros, DROP COLUMN provider_calls;
