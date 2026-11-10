-- Transfers between registered users are account movements, not
-- expense/income. Counterparty display name lives in description JSONB
-- ({"transferTo":"Dafina Balqis"}) to avoid touching every query.
ALTER TABLE transaction_log DROP CONSTRAINT transaction_log_direction_check;
ALTER TABLE transaction_log ADD CONSTRAINT transaction_log_direction_check
  CHECK (direction IN ('expense','income','transfer'));
