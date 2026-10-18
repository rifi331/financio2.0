-- Extend the category set with the additional categories used by Financio 1.0.
-- Existing rows are untouched (names merged to the singular forms seeded in 0003).
INSERT INTO category (name) VALUES
  ('Sport Equipments'),
  ('Snacks'),
  ('Lifestyles'),
  ('Fuel & Gas'),
  ('Drinks'),
  ('Clothings'),
  ('Furnitures'),
  ('Car Maintenances')
ON CONFLICT (name) DO NOTHING;
