-- +goose Up

-- Profile fields used to personalise campaigns.
--   language:   the language this person receives messages in (tr | en | de).
--               NOT NULL with a default so existing rows stay valid.
--   gender:     optional; NULL means "not stated".
--   birth_date: optional; NULL means "not stated".
ALTER TABLE users
    ADD COLUMN language   TEXT NOT NULL DEFAULT 'tr'
        CONSTRAINT users_language_check CHECK (language IN ('tr', 'en', 'de')),
    ADD COLUMN gender     TEXT
        CONSTRAINT users_gender_check CHECK (gender IN ('female', 'male', 'other')),
    ADD COLUMN birth_date DATE;

-- +goose Down
ALTER TABLE users DROP COLUMN birth_date, DROP COLUMN gender, DROP COLUMN language;
