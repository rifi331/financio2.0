-- /options command catalog: every feature and how to use it, editable in DB.
CREATE TABLE IF NOT EXISTS option_help (
  command     TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  section     TEXT NOT NULL DEFAULT 'General',
  sort_order  SMALLINT NOT NULL DEFAULT 0
);

INSERT INTO option_help (command, description, section, sort_order) VALUES
  ('10rm nasi lemak', 'Log an expense (text). Also: ''gaji 6500'' for income, ''nasi lemak 10rm yesterday'', ''rp50k kopi''.', 'Entry', 1),
  ('photo', 'Send a receipt or statement screenshot — every row becomes a transaction (multi-row OK).', 'Entry', 2),
  ('statement.pdf', 'Send a bank/e-wallet statement PDF as a document — spending rows are parsed, top-up rows skipped.', 'Entry', 3),
  ('edit <id> amount 12.50 [category X] [item label] [date 2026-10-01] [currency MYR]', 'Fix a transaction (fields optional, combinable).', 'Manage', 10),
  ('delete <id> / hapus <id>', 'Delete one of your transactions by id.', 'Manage', 11),
  ('check duplicate <month> / cek duplikat <bulan>', 'List same item+amount+date entries with accidental/same-day hints (you decide what to delete).', 'Manage', 12),
  ('recap <period> [category] [order by category]', 'Spending summary. E.g. ''recap oct'', ''recap this week meals''.', 'Reports', 20),
  ('full report <period> [category] [order by category]', 'PDF report: summary, pie+bar charts, budgets, month comparison.', 'Reports', 21),
  ('/budget set <category> <limit> [currency]', 'Monthly budget for your group. Also: /budget list, /budget delete <category>.', 'Budgets', 30),
  ('(automatic)', 'Budget alert at 80% and 100% of a budget, once a month.', 'Budgets', 31),
  ('/group list / /group set <name> <group>', 'Budget groups: who shares a budget with whom.', 'Budgets', 32),
  ('/recurring add <item> <amount> [day N] [category X] [income]', 'Monthly auto-entries. Also: /recurring list, /recurring delete <id>.', 'Automation', 40),
  ('/prompt list|show|set|reset <name>', 'Edit AI prompts live (entry, recap_filter, photo_receipt, pdf_receipt).', 'Admin', 50),
  ('/param list|show|set <key>', 'Tune runtime params: openai_model (immediate), AI/message timeouts (on restart).', 'Admin', 51),
  ('/options', 'This menu.', 'General', 60),
  ('/start, /help', 'Quick bilingual usage card.', 'General', 61)
ON CONFLICT (command) DO NOTHING;
