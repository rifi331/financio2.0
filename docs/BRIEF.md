# Financio 2.0 — Shared Brief (authoritative)

Every build session reads this file first. It is the single source of truth for scope,
stack, schema, configuration, and runtime contract. Do not modify unless the user asks.

## Scope

1.0 parity (text entry, delete, recap ± category, photo receipt, PDF statement) plus:
income tracking, a currency table, and an allowlist. Deviations from 1.0, all deliberate:

| 1.0 (n8n) | 2.0 (Go) | Why |
|---|---|---|
| AI writes recap SQL, executed raw | AI returns `{dateFrom, dateTo, categories[], language}`; Go builds **parameterized** SQL | never execute model-generated SQL |
| identity = `first_name + last_name` string on every row | `app_user` keyed by Telegram user ID, allowlist by display name, ID auto-bound on first message | names can change/duplicate; one row per human |
| recap total sums mixed currencies under one label | totals grouped **per currency** (`MYR 123.40 + IDR 250.000`) | correctness |
| expense only (income hints ignored) | `direction` column: `expense`/`income`, AI-detected (`gaji`, `salary`, `bonus`, `received …`) | user requirement |
| triplicated insert paths (text/photo/pdf) | one `InsertTransaction` used by all sources | maintainability |
| n8n + Spring Boot webhook + ingress | one Go binary, **long polling** (outbound-only), no ingress needed | lightweight, TrueNAS-friendly |

## Stack (fixed; do not deviate without reason)

Go 1.24+, `go-telegram-bot-api/v5` (long polling), `jackc/pgx/v5` + pgxpool, OpenAI via
plain `net/http` (chat completions with `response_format: json_schema` — structured
outputs), hand-rolled embedded SQL migrations (`embed.FS` + `schema_migrations` table),
`ledongthuc/pdf` for PDF text extraction, `log/slog` for logs, stdlib `testing` +
`httptest` for tests. No web framework, no ORM, no OpenAI SDK.

## Runtime contract

Single process, single replica (long polling holds the `getUpdates` lock — never scale
past 1). Timezone `Asia/Kuala_Lumpur` (configurable). Currencies seeded: IDR, MYR, USD,
SGD (extensible via table). Categories seeded from 1.0: Groceries, Medical, Utilities,
Housing, Meals, Entertainment, Living Expenses, Education, Public Transportation, Others.
Extended set (user, 2026-10-18; singular forms canonical): + Sport Equipments, Snacks,
Lifestyles, Fuel & Gas, Drinks, Clothings, Furnitures, Car Maintenances.
Default currency MYR unless the message says otherwise (`Rp` → IDR, `rm` → MYR, `s$`/`sgd`
→ SGD, `$`/`usd` → USD). Languages: English and Bahasa Indonesia.

## Secrets policy (hard rule)

No API key, bot token, password, or DSN may ever be committed to this git repo — not in
code, comments, test fixtures, example files, log captures, or filled-in manifests.
Git history is permanent: anything committed once stays recoverable even after deletion,
and the only safe remedy for a leaked secret is rotation. Therefore:

- All secrets enter at runtime via the env vars under "Config"; `config.String()` must
  redact them from logs.
- The deploy manifest ships with placeholders only; real values are substituted at apply
  time, never saved to a tracked file.
- Manual test runs with a real token use an untracked `.env` (gitignored).
- If a secret is accidentally committed anyway: rotate it immediately, then rewrite
  history if the repo was already pushed.

## Schema

```sql
CREATE TABLE app_user (
  id            BIGINT PRIMARY KEY,              -- telegram user id, bound on first message
  display_name  TEXT NOT NULL,                   -- "First Last"
  is_active     BOOLEAN NOT NULL DEFAULT TRUE,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE currency (
  code            TEXT PRIMARY KEY,               -- MYR, IDR, USD, SGD
  symbol          TEXT NOT NULL,                  -- RM, Rp, $, S$
  decimal_places  SMALLINT NOT NULL DEFAULT 2,    -- IDR => 0
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
  description   JSONB,                            -- {"details":[{item,amount,qty,location}]}
  source        TEXT NOT NULL DEFAULT 'text' CHECK (source IN ('text','photo','pdf')),
  raw_message   TEXT,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_txn_user_date ON transaction_log (user_id, entry_date);
```

## Config (env, read in `internal/config`)

```
TELEGRAM_BOT_TOKEN   (required)
OPENAI_API_KEY       (required)
DATABASE_URL         (required, e.g. postgres://user:pass@host:5432/financio)
OPENAI_MODEL         (default gpt-6-luna; overridable via sysparam openai_model)
ALLOWED_USERS        (required, comma-separated display names: "Rifi Anggriawan,Jane Doe")
DISPLAY_TZ           (default Asia/Kuala_Lumpur)
DEFAULT_CURRENCY     (default MYR)
HEALTH_PORT          (default 7777)
LOG_LEVEL            (default info)
```
