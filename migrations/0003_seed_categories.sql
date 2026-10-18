INSERT INTO category (name) VALUES
  ('Groceries'),
  ('Medical'),
  ('Utilities'),
  ('Housing'),
  ('Meals'),
  ('Entertainment'),
  ('Living Expenses'),
  ('Education'),
  ('Public Transportation'),
  ('Others')
ON CONFLICT (name) DO NOTHING;
