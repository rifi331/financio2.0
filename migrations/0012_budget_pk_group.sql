-- Budgets are per-group: the primary key must include the group, otherwise
-- group B's budget for Meals/MYR silently overwrites group A's.
ALTER TABLE budget DROP CONSTRAINT budget_pkey;
ALTER TABLE budget ADD CONSTRAINT budget_pkey
  PRIMARY KEY (budget_group_id, category_id, currency_code);
