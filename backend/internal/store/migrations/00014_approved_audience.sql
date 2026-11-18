-- +goose Up

-- The reachable recipients the screen showed when the campaign was approved
-- (sms_reachable / email_reachable at that moment). The sending screen compares
-- them with the deliveries actually queued and says so only when they differ.
-- A scheduled campaign recounts sms_reachable / email_reachable when it starts,
-- so the approval's numbers are kept here. NULL for campaigns approved before.
ALTER TABLE campaigns ADD COLUMN approved_sms INT, ADD COLUMN approved_email INT;

-- +goose Down
ALTER TABLE campaigns DROP COLUMN approved_email, DROP COLUMN approved_sms;
