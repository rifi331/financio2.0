-- Runtime-tunable system parameters, editable via the /param bot command.
-- Secrets (tokens, DSN, allowlist) never live here — env only.
CREATE TABLE IF NOT EXISTS sysparam (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO sysparam (key, value) VALUES
  ('openai_model', 'gpt-6-luna'),
  ('ai_attempt_timeout_seconds', '80'),
  ('message_timeout_seconds', '300')
ON CONFLICT (key) DO NOTHING;
