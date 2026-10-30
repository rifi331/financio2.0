-- Recurring monthly transactions (auto-inserted by the daily runner).
CREATE TABLE IF NOT EXISTS recurring (
  id            BIGSERIAL PRIMARY KEY,
  user_id       BIGINT NOT NULL REFERENCES app_user(id),
  item          TEXT NOT NULL,
  category_name TEXT NOT NULL,
  currency_code TEXT NOT NULL REFERENCES currency(code),
  amount        NUMERIC(14,2) NOT NULL CHECK (amount > 0),
  direction     TEXT NOT NULL DEFAULT 'expense' CHECK (direction IN ('expense','income')),
  day_of_month  SMALLINT NOT NULL DEFAULT 1 CHECK (day_of_month BETWEEN 1 AND 31),
  last_run      DATE,
  is_active     BOOLEAN NOT NULL DEFAULT TRUE,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The runner inserts with source 'recurring'.
ALTER TABLE transaction_log DROP CONSTRAINT transaction_log_source_check;
ALTER TABLE transaction_log ADD CONSTRAINT transaction_log_source_check
  CHECK (source IN ('text','photo','pdf','recurring'));
