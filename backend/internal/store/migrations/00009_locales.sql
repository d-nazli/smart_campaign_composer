-- +goose Up

-- Locales replace the fixed tr/en/de list. A locale is a language for one
-- market (BCP 47: en-GB, es-MX). Adding a locale is adding a row here: the
-- code reads this table, nothing lists the locales in Go.
--   is_source:           the locale every campaign is written in first; the
--                        others are translated from it. Exactly one row.
--   fallbacks:           the word a variable becomes when the person's value
--                        is missing or too long ({"first_name": "valued customer"}).
--   free_shipping_terms: how this locale writes "free shipping", so a
--                        translation can be checked against the source.
CREATE TABLE locales (
    code                TEXT PRIMARY KEY CONSTRAINT locales_code_check CHECK (code ~ '^[a-z]{2,3}-[A-Z]{2}$'),
    name                TEXT    NOT NULL,
    native_name         TEXT    NOT NULL,
    direction           TEXT    NOT NULL DEFAULT 'ltr' CONSTRAINT locales_direction_check CHECK (direction IN ('ltr', 'rtl')),
    is_source           BOOLEAN NOT NULL DEFAULT false,
    fallbacks           JSONB   NOT NULL DEFAULT '{}' CONSTRAINT locales_fallbacks_check CHECK (jsonb_typeof(fallbacks) = 'object'),
    free_shipping_terms TEXT[]  NOT NULL DEFAULT '{}'
);
CREATE UNIQUE INDEX locales_one_source ON locales (is_source) WHERE is_source;

INSERT INTO locales (code, name, native_name, direction, is_source, fallbacks, free_shipping_terms) VALUES
('tr-TR', 'Turkish (Türkiye)',        'Türkçe (Türkiye)',         'ltr', true,  '{"first_name": "değerli müşterimiz", "city": "bölgeniz"}',
    '{"ücretsiz kargo", "kargo ücretsiz", "bedava kargo", "kargo bedava", "ücretsiz gönderim", "ücretsiz teslimat"}'),
('en-GB', 'English (United Kingdom)', 'English (UK)',             'ltr', false, '{"first_name": "valued customer", "city": "your area"}',
    '{"free shipping", "free delivery", "delivery is free", "free postage"}'),
('en-US', 'English (United States)',  'English (US)',             'ltr', false, '{"first_name": "valued customer", "city": "your area"}',
    '{"free shipping", "free delivery", "shipping is free", "ships free"}'),
('en-AU', 'English (Australia)',      'English (Australia)',      'ltr', false, '{"first_name": "valued customer", "city": "your area"}',
    '{"free shipping", "free delivery", "shipping is free", "free postage"}'),
('de-DE', 'German (Germany)',         'Deutsch (Deutschland)',    'ltr', false, '{"first_name": "geschätzter Kunde", "city": "Ihrer Region"}',
    '{"kostenloser versand", "kostenlosen versand", "kostenlose lieferung", "kostenfreier versand", "versandkostenfrei", "gratis versand"}'),
('fr-FR', 'French (France)',          'Français (France)',        'ltr', false, '{"first_name": "cher client", "city": "votre région"}',
    '{"livraison gratuite", "livraison offerte", "frais de port offerts"}'),
('es-ES', 'Spanish (Spain)',          'Español (España)',         'ltr', false, '{"first_name": "estimado cliente", "city": "tu zona"}',
    '{"envío gratis", "envío gratuito", "gastos de envío gratis"}'),
('es-MX', 'Spanish (Mexico)',         'Español (México)',         'ltr', false, '{"first_name": "estimado cliente", "city": "tu zona"}',
    '{"envío gratis", "envío gratuito"}'),
('it-IT', 'Italian (Italy)',          'Italiano (Italia)',        'ltr', false, '{"first_name": "gentile cliente", "city": "la tua zona"}',
    '{"spedizione gratuita", "spedizione gratis"}'),
('nl-NL', 'Dutch (Netherlands)',      'Nederlands (Nederland)',   'ltr', false, '{"first_name": "beste klant", "city": "jouw regio"}',
    '{"gratis verzending", "gratis bezorging", "verzending gratis"}'),
('pt-BR', 'Portuguese (Brazil)',      'Português (Brasil)',       'ltr', false, '{"first_name": "cliente especial", "city": "sua região"}',
    '{"frete grátis", "frete gratuito", "entrega grátis"}'),
('ar-SA', 'Arabic (Saudi Arabia)',    'العربية (السعودية)',       'rtl', false, '{"first_name": "عميلنا العزيز", "city": "منطقتك"}',
    '{"شحن مجاني", "توصيل مجاني", "الشحن مجاني", "التوصيل مجاني"}');

-- users.language (tr | en | de) becomes users.locale, a reference to locales.
ALTER TABLE users ADD COLUMN locale TEXT CONSTRAINT users_locale_fkey REFERENCES locales (code);
UPDATE users SET locale = CASE language WHEN 'en' THEN 'en-GB' WHEN 'de' THEN 'de-DE' ELSE 'tr-TR' END;
ALTER TABLE users ALTER COLUMN locale SET NOT NULL, ALTER COLUMN locale SET DEFAULT 'tr-TR';
ALTER TABLE users DROP COLUMN language;
CREATE INDEX users_locale_idx ON users (locale);

-- One text per campaign and locale. The source row counts its versions; a
-- translation remembers which source version it was made from
-- (source_version < source's version means "not up to date") and that source
-- text itself (source_text), so the screen can show what changed since.
ALTER TABLE campaign_contents
    ADD COLUMN locale TEXT CONSTRAINT campaign_contents_locale_fkey REFERENCES locales (code),
    ADD COLUMN version INT NOT NULL DEFAULT 1 CONSTRAINT campaign_contents_version_check CHECK (version >= 1),
    ADD COLUMN source_version INT,
    ADD COLUMN source_text JSONB;
UPDATE campaign_contents SET locale = CASE language WHEN 'en' THEN 'en-GB' WHEN 'de' THEN 'de-DE' ELSE 'tr-TR' END;
UPDATE campaign_contents t SET source_version = 1,
       source_text = (SELECT jsonb_build_object('sms_body', s.sms_body, 'email_subject', s.email_subject, 'email_body', s.email_body)
                      FROM campaign_contents s WHERE s.campaign_id = t.campaign_id AND s.locale = 'tr-TR')
WHERE t.locale <> 'tr-TR';
ALTER TABLE campaign_contents DROP CONSTRAINT campaign_contents_source_check;
UPDATE campaign_contents SET source = 'translated' WHERE locale <> 'tr-TR' AND source <> 'edited';
ALTER TABLE campaign_contents ADD CONSTRAINT campaign_contents_source_check
    CHECK (source IN ('llm', 'fallback', 'edited', 'translated'));
ALTER TABLE campaign_contents DROP CONSTRAINT campaign_contents_pkey;
ALTER TABLE campaign_contents ALTER COLUMN locale SET NOT NULL;
ALTER TABLE campaign_contents DROP COLUMN language;
ALTER TABLE campaign_contents ADD PRIMARY KEY (campaign_id, locale);

ALTER TABLE llm_generations ADD COLUMN locale TEXT CONSTRAINT llm_generations_locale_fkey REFERENCES locales (code);
UPDATE llm_generations SET locale = CASE language WHEN 'en' THEN 'en-GB' WHEN 'de' THEN 'de-DE' ELSE 'tr-TR' END;
ALTER TABLE llm_generations ALTER COLUMN locale SET NOT NULL, ALTER COLUMN locale SET DEFAULT 'tr-TR';
ALTER TABLE llm_generations DROP COLUMN language;
-- Translation attempts are audited like the others.
ALTER TABLE llm_generations DROP CONSTRAINT llm_generations_kind_check;
ALTER TABLE llm_generations ADD CONSTRAINT llm_generations_kind_check CHECK (kind IN ('segment', 'messages', 'translate'));

-- How many reachable people each locale has, e.g. {"tr-TR": 120, "en-GB": 45}.
-- The locales in it (plus the source) are the ones the campaign is written in.
ALTER TABLE campaigns RENAME COLUMN language_counts TO locale_counts;
UPDATE campaigns SET locale_counts = (
    SELECT jsonb_object_agg(CASE key WHEN 'en' THEN 'en-GB' WHEN 'de' THEN 'de-DE' ELSE 'tr-TR' END, value)
    FROM jsonb_each(locale_counts))
WHERE locale_counts IS NOT NULL AND locale_counts <> '{}'::jsonb;

-- The locale a delivery was written in, set when it is claimed for sending.
ALTER TABLE deliveries ADD COLUMN locale TEXT CONSTRAINT deliveries_locale_fkey REFERENCES locales (code);

-- +goose Down
ALTER TABLE deliveries DROP COLUMN locale;

UPDATE campaigns SET locale_counts = (
    SELECT jsonb_object_agg(k, n) FROM (
        SELECT CASE WHEN key LIKE 'en-%' THEN 'en' WHEN key LIKE 'de-%' THEN 'de' ELSE 'tr' END AS k, sum(value::int) AS n
        FROM jsonb_each(locale_counts) GROUP BY 1) t)
WHERE locale_counts IS NOT NULL AND locale_counts <> '{}'::jsonb;
ALTER TABLE campaigns RENAME COLUMN locale_counts TO language_counts;

DELETE FROM llm_generations WHERE kind = 'translate';
ALTER TABLE llm_generations DROP CONSTRAINT llm_generations_kind_check;
ALTER TABLE llm_generations ADD CONSTRAINT llm_generations_kind_check CHECK (kind IN ('segment', 'messages'));
ALTER TABLE llm_generations ADD COLUMN language TEXT NOT NULL DEFAULT 'tr';
UPDATE llm_generations SET language = CASE WHEN locale LIKE 'en-%' THEN 'en' WHEN locale LIKE 'de-%' THEN 'de' ELSE 'tr' END;
ALTER TABLE llm_generations DROP COLUMN locale;

-- Only tr-TR, en-GB and de-DE map back to the old languages; the rest is dropped.
DELETE FROM campaign_contents WHERE locale NOT IN ('tr-TR', 'en-GB', 'de-DE');
ALTER TABLE campaign_contents ADD COLUMN language TEXT NOT NULL DEFAULT 'tr'
    CONSTRAINT campaign_contents_language_check CHECK (language IN ('tr', 'en', 'de'));
UPDATE campaign_contents SET language = CASE locale WHEN 'en-GB' THEN 'en' WHEN 'de-DE' THEN 'de' ELSE 'tr' END;
ALTER TABLE campaign_contents DROP CONSTRAINT campaign_contents_source_check;
UPDATE campaign_contents SET source = 'llm' WHERE source = 'translated';
ALTER TABLE campaign_contents ADD CONSTRAINT campaign_contents_source_check CHECK (source IN ('llm', 'fallback', 'edited'));
ALTER TABLE campaign_contents DROP CONSTRAINT campaign_contents_pkey;
ALTER TABLE campaign_contents ADD PRIMARY KEY (campaign_id, language);
ALTER TABLE campaign_contents DROP COLUMN locale, DROP COLUMN version, DROP COLUMN source_version, DROP COLUMN source_text;

ALTER TABLE users ADD COLUMN language TEXT NOT NULL DEFAULT 'tr'
    CONSTRAINT users_language_check CHECK (language IN ('tr', 'en', 'de'));
UPDATE users SET language = CASE WHEN locale LIKE 'en-%' THEN 'en' WHEN locale LIKE 'de-%' THEN 'de' ELSE 'tr' END;
DROP INDEX users_locale_idx;
ALTER TABLE users DROP COLUMN locale;

DROP TABLE locales;
