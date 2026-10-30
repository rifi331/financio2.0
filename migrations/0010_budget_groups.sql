-- Budget groups: budgets belong to a group of users instead of the household.
CREATE TABLE IF NOT EXISTS budget_group (
  id         BIGSERIAL PRIMARY KEY,
  name       TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE app_user ADD COLUMN budget_group_id BIGINT REFERENCES budget_group(id);

-- Seed: one "Family" group containing every existing user; existing budgets
-- move into it (they were household-wide until now).
INSERT INTO budget_group (name) SELECT 'Family'
WHERE NOT EXISTS (SELECT 1 FROM budget_group WHERE name = 'Family');
UPDATE app_user SET budget_group_id = (SELECT id FROM budget_group WHERE name = 'Family')
WHERE budget_group_id IS NULL;

ALTER TABLE budget ADD COLUMN budget_group_id BIGINT REFERENCES budget_group(id);
UPDATE budget SET budget_group_id = (SELECT id FROM budget_group WHERE name = 'Family')
WHERE budget_group_id IS NULL;
ALTER TABLE budget ALTER COLUMN budget_group_id SET NOT NULL;
