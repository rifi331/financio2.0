INSERT INTO currency (code, symbol, decimal_places, sort_order) VALUES
  ('MYR', 'RM', 2, 1),
  ('IDR', 'Rp', 0, 2),
  ('USD', '$', 2, 3),
  ('SGD', 'S$', 2, 4)
ON CONFLICT (code) DO NOTHING;
