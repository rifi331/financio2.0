# Financio 2.0

A private Telegram expense/income tracker: one lightweight Go binary, long-polling
Telegram (no ingress), Postgres for storage, GPT-4o-mini for structured parsing.
Replaces the n8n "Financio postgres" automation.

## Features

- **Text entry**: `10rm nasi lemak`, `nasi lemak 10rm yesterday`, `gaji 6500`
  (AI-detected income) — insert via one shared, parameterized path.
- **Delete**: `hapus 123` / `delete 123` (ownership-scoped, no leakage).
- **Recap**: `recap nov 2025`, `recap this week meals`, `rekap bulan ini` —
  the AI returns a structured filter only; Go builds the SQL.
- **Receipts**: send a **photo** of a receipt (vision parsing; one transaction
  with line-item details) or a **bank-statement PDF** (text extraction with a
  30KB guard; spending rows kept, wallet top-up "Receive" rows skipped).
- **Duplicate check**: `check duplicate oct` / `cek duplikat oktober` — lists
  same item+amount+date groups, with ids ready for `delete <id>`.
- **Full report**: `full report oct 2026` / `laporan lengkap oktober` — PDF with
  summary, per-currency expense pie charts, and income/expense tables.
- **Web dashboard**: token-gated read-only UI on `HEALTH_PORT` — transactions
  with filters, per-currency net totals, live log tail (`DASHBOARD_TOKEN`).
- **Help**: `/start`, `/help` — bilingual usage card.
- **Hardened**: panic in any handler is recovered and logged; 90s per-message
  timeout; at most 4 messages processed concurrently with per-chat ordering;
  Postgres outages degrade to error replies (poll loop survives); Telegram 409
  (second poller) exits loudly — single replica only.

## Configuration (env)

| Variable | Required | Default | Notes |
|---|---|---|---|
| `TELEGRAM_BOT_TOKEN` | ✅ | | from @BotFather |
| `OPENAI_API_KEY` | ✅ | | |
| `DATABASE_URL` | ✅ | | `postgres://user:pass@host:5432/financio` |
| `ALLOWED_USERS` | ✅ | | comma-separated display names, e.g. `Rifi Anggriawan` |
| `OPENAI_MODEL` | | `gpt-6-luna` | |
| `DISPLAY_TZ` | | `Asia/Kuala_Lumpur` | |
| `DEFAULT_CURRENCY` | | `MYR` | seeded: MYR, IDR, USD, SGD |
| `HEALTH_PORT` | | `7777` | `/healthz` |
| `LOG_LEVEL` | | `info` | JSON logs via slog |
| `DASHBOARD_TOKEN` | | disabled | enables read-only web dashboard on `HEALTH_PORT` (transactions + log tail) |

Secrets enter at runtime only — never committed (see `docs/BRIEF.md`).

## Run locally

```sh
make build          # binary in dist/financio, build info via -ldflags
make test           # go test ./...
make lint           # go vet ./...

# with a local Postgres and an untracked .env (gitignored):
export $(grep -v '^#' .env | xargs) && make run
curl localhost:7777/healthz
```

Migrations are embedded and applied automatically at startup. Store tests are
gated behind `FINANCIO_TEST_DSN`.

## Deploy

Docker image (`make image` → `financio:2.0.0`, distroless, non-root, < 30 MB) and
three deploy paths — TrueNAS Custom App via ghcr (preferred), kubectl apply on
k3s, or airgapped image import — see [deploy/README.md](deploy/README.md).
Single replica only (long polling); outbound-only, no Service/Ingress.

## Repository layout

`cmd/financio` (wiring) · `internal/config` · `internal/store` (pgx + migrations)
· `internal/telegram` (long polling, sends, downloads) · `internal/router`
(intent classify) · `internal/auth` (allowlist) · `internal/ai` (chat
completions + structured outputs) · `internal/bot` (dispatcher, worker pool)
· `internal/recap` (formatter) · `internal/receipt` (PDF extraction + helpers).

Build-prompt series lives in `prompts/`; progress in `status.md`; scope and
schema in `docs/BRIEF.md`.

## Phone notification ingestion (MacroDroid)

Payments can be logged automatically from app notifications (TNG eWallet, GPay,
RHB, myBCA) without touching Telegram:

1. Set `DASHBOARD_TOKEN` (already required for the web dashboard).
2. In MacroDroid, create a macro:
   - **Trigger**: Notification Received → select the payment apps → Text
     content, enable regex, use e.g. `(?i)paid|payment|transaction|bayar|rm|spent`
     (⚠️ the field is **regex**, NOT a comma-separated list — a comma list is
     matched as one literal string and matches nothing)
   - **Actions**:
     1. *(optional)* Toast Message `FIRED: {not_title}: {notification}` —
        shows exactly what was captured
     2. HTTP Request → POST `https://financio.parifidev.com/ingest`
        (Cloudflare tunnel) or `http://<truenas-ip>:7777/ingest` (LAN)
        - Header: `X-Token: <DASHBOARD_TOKEN>`
        - Content Body type **Text**, body: `{not_title}: {notification}`
        (plain text is accepted and needs no JSON escaping — quotes in
        notification text break the JSON form)
3. Exempt MacroDroid from battery optimization, or night-time notifications
   will be missed.

The text is parsed like a typed message; confirmations arrive in Telegram.
Ingested entries are attributed to the primary user (first `ALLOWED_USERS` name).

## Troubleshooting

| Symptom | Cause / fix |
|---|---|
| Every AI command fails (`Could not parse that`, recap/report errors) | OpenAI call rejected: check credit balance, key validity (`OPENAI_API_KEY` env), or model (`/param list`; `/param set openai_model gpt-4o-mini`). DB-only commands (`/options`, `/budget list`) working while AI fails = key/credit problem. |
| PDF details show raw JSON instead of a table | Old renderer build — description JSONB has two shapes (2.0 bare array, 1.0 `{"details":...}`); needs v2.8.4+. |
| Report pie is one solid color / legends overlap | Two currencies drawn on one page — fixed in v2.9.2 (per-currency chart pages). |
| `check duplicate` fails on every month | Fixed in v2.8.6 (window function in WHERE was illegal SQL). |
| MacroDroid posts `{notification_title}: ...` literally | Wrong magic-text names — use `{not_title}` and `{notification}` from the {} picker, and regex text matching (see above). |
| Rotated a credential and the bot broke | Expected — put the new value in the TrueNAS app env and recreate. |

## Verifying / inspecting data

```sh
# recent transactions incl. description JSON
sudo docker exec <pg-container> psql -U postgres -d financio \
  -c "select id, item, direction, description from financio2.transaction_log order by created_at desc limit 5;"
# migration level
sudo docker exec <pg-container> psql -U postgres -d financio \
  -c "select * from financio2.schema_migrations order by id;"
```

Dashboard (`/txn/<id>`) shows the same detail per transaction.
