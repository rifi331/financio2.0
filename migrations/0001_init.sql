CREATE TABLE app_user (
  id            BIGINT PRIMARY KEY,
  display_name  TEXT NOT NULL,
  is_active     BOOLEAN NOT NULL DEFAULT TRUE,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE currency (
  code            TEXT PRIMARY KEY,
  symbol          TEXT NOT NULL,
  decimal_places  SMALLINT NOT NULL DEFAULT 2,
  is_active       BOOLEAN NOT NULL DEFAULT TRUE,
  sort_order      SMALLINT NOT NULL DEFAULT 0
);

CREATE TABLE category (
  id        BIGSERIAL PRIMARY KEY,
  name      TEXT NOT NULL UNIQUE,
  is_active BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE transaction_log (
  id            BIGSERIAL PRIMARY KEY,
  user_id       BIGINT NOT NULL REFERENCES app_user(id),
  item          TEXT NOT NULL,
  category_id   BIGINT NOT NULL REFERENCES category(id),
  currency_code TEXT NOT NULL REFERENCES currency(code),
  total_amount  NUMERIC(14,2) NOT NULL CHECK (total_amount > 0),
  direction     TEXT NOT NULL DEFAULT 'expense' CHECK (direction IN ('expense','income')),
  entry_date    DATE NOT NULL,
  description   JSONB,
  source        TEXT NOT NULL DEFAULT 'text' CHECK (source IN ('text','photo','pdf')),
  raw_message   TEXT,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_txn_user_date ON transaction_log (user_id, entry_date);
