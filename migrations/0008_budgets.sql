-- Monthly spending budgets per category and currency (household-wide).
-- A budget applies to the calendar month of a transaction's entry_date.
CREATE TABLE IF NOT EXISTS budget (
  category_id   BIGINT NOT NULL REFERENCES category(id),
  currency_code TEXT   NOT NULL REFERENCES currency(code),
  monthly_limit NUMERIC(14,2) NOT NULL CHECK (monthly_limit > 0),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (category_id, currency_code)
);
