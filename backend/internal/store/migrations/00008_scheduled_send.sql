-- +goose Up

-- Scheduled sending. A campaign approved "for later" is stored as it is and
-- nothing is queued: when its time comes the worker runs the stored segment
-- rule again, takes the audience of THAT moment and creates the deliveries.
--   draft -> segment_ready -> content_ready -> scheduled -> approved -> sending -> completed
--                                                  |-> cancelled   (by a person, before it starts)
--                                                  '-> failed      (cannot be sent any more)
ALTER TABLE campaigns
    ADD COLUMN scheduled_at    TIMESTAMPTZ,
    -- When the worker built the audience and queued the deliveries.
    ADD COLUMN materialized_at TIMESTAMPTZ,
    ADD COLUMN cancelled_at    TIMESTAMPTZ,
    -- Why a scheduled campaign failed (shown on the dashboard).
    ADD COLUMN schedule_error  TEXT NOT NULL DEFAULT '';

ALTER TABLE campaigns DROP CONSTRAINT campaigns_status_check;
ALTER TABLE campaigns ADD CONSTRAINT campaigns_status_check CHECK (status IN
    ('draft', 'segment_ready', 'content_ready', 'scheduled', 'approved', 'sending', 'completed', 'cancelled', 'failed'));

-- The scheduler looks for the campaigns that are due.
CREATE INDEX campaigns_due_idx ON campaigns (scheduled_at) WHERE status = 'scheduled';

-- +goose Down
DROP INDEX campaigns_due_idx;
-- Campaigns that never started go back to "waiting for approval".
UPDATE campaigns SET status = 'content_ready' WHERE status IN ('scheduled', 'cancelled', 'failed');
ALTER TABLE campaigns DROP CONSTRAINT campaigns_status_check;
ALTER TABLE campaigns ADD CONSTRAINT campaigns_status_check CHECK (status IN
    ('draft', 'segment_ready', 'content_ready', 'approved', 'sending', 'completed'));
ALTER TABLE campaigns DROP COLUMN schedule_error, DROP COLUMN cancelled_at, DROP COLUMN materialized_at, DROP COLUMN scheduled_at;
