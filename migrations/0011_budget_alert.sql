-- Alert-once-per-month marker for budget threshold notifications.
ALTER TABLE budget ADD COLUMN last_alert_month TEXT;
