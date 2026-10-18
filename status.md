# Financio 2.0 — Build Status

Update this file after every completed prompt session: set Status to `Done`, fill in the
date, and note anything deviated from the prompt (skipped items, decisions made).

Legend: `Pending` → `In Progress` → `Done` (or `Skipped` with a reason).

| # | Prompt file | Scope | Status | Session date | Notes |
|---|---|---|---|---|---|
| 1 | `prompts/1-foundation.md` | Repo bootstrap, config, DB schema, migrations, store | Done | 2026-10-18 | All items done. Placeholder-package layout in place for sessions 2–4. |
| 2 | `prompts/2-bot-and-entry.md` | Telegram polling, allowlist auth, router, AI text-entry flow | Pending | | |
| 3 | `prompts/3-delete-and-recap.md` | Delete + recap flows | Pending | | |
| 4 | `prompts/4-receipts-and-hardening.md` | Photo/PDF receipts, /start & /help, resilience | Pending | | |
| 5 | `prompts/5-deploy.md` | Docker image, TrueNAS k3s manifest, final sweep | Pending | | |
| 6 | `prompts/6-optional-import-v1.md` | Optional: import 1.0 n8n-era data | Pending | | |
| — | `verification.md` | End-to-end manual verification checklist | Pending | | |

## Session log

### Session 1 — 2026-10-18
- Part A (stdlib only): config with fail-fast + redacted `String()`, slog JSON logging,
  `/healthz` server on HEALTH_PORT with graceful shutdown, Makefile, git init.
- Part B: pgx/v5, hand-rolled migrations (`migrations/*.sql` embedded via a small
  `migrations` package's `embed.FS` — embed can't reach parent dirs from `internal/store`,
  so `store.Migrate(ctx, pool, migrations.FS)` takes an `fs.FS`). 0001 schema per BRIEF,
  0002 currency seeds, 0003 category seeds.
- Store methods: EnsureUser (upsert w/ name-change update), GetUserByTelegramID,
  ListActiveCategories/Currencies, InsertTransaction (single INSERT..SELECT..RETURNING,
  typed `ErrUnknownCategory`), DeleteTransactionByID (ownership via WHERE user_id),
  ListTransactions (dynamic WHERE, categories via `= ANY($n)`; one `fmt.Sprintf` used only
  to compute placeholder numbers, never SQL text).
- Store tests gated behind `FINANCIO_TEST_DSN` (skip when unset) — they cover migration
  idempotency, seeds, upsert, unknown category, delete ownership, list filters.
  Not yet run against a real DB; run with `FINANCIO_TEST_DSN=... go test ./internal/store/`.
- `TotalAmount` scanned as string (NUMERIC) to avoid float rounding; session 2+ code should
  format amounts from the string.
- Watch out for session 2: migrations are applied via `store.Migrate(ctx, pool, migrations.FS)`
  in `main.go` once the pool is wired up.
