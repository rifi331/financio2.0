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
| `OPENAI_MODEL` | | `gpt-4o-mini` | |
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
