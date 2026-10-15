ALTER TABLE applications ADD COLUMN IF NOT EXISTS last_pulled_at   TIMESTAMPTZ;
ALTER TABLE applications ADD COLUMN IF NOT EXISTS last_pull_status  VARCHAR(50);
ALTER TABLE applications ADD COLUMN IF NOT EXISTS last_pull_message TEXT;
