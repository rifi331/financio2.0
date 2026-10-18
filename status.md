# Financio 2.0 — Build Status

Update this file after every completed prompt session: set Status to `Done`, fill in the
date, and note anything deviated from the prompt (skipped items, decisions made).

Legend: `Pending` → `In Progress` → `Done` (or `Skipped` with a reason).

| # | Prompt file | Scope | Status | Session date | Notes |
|---|---|---|---|---|---|
| 1 | `prompts/1-foundation.md` | Repo bootstrap, config, DB schema, migrations, store | Done | 2026-10-18 | All items done. Placeholder-package layout in place for sessions 2–4. |
| 2 | `prompts/2-bot-and-entry.md` | Telegram polling, allowlist auth, router, AI text-entry flow | Done | 2026-10-18 | All items done. Dispatcher lives in new `internal/bot` package. |
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

### Session 2 — 2026-10-18
- Telegram: `internal/telegram.Client` — long polling (60s timeout, messages only), plain
  text sends, 4000-char splitting. Auth: `auth.Authorizer.Authorize(ctx, id, first, last)`
  — known active users pass; unknown senders pass only if display name matches
  ALLOWED_USERS (normalized, case-insensitive), binding the telegram ID via EnsureUser;
  others dropped silently (info log, no reply).
- Router: `router.Classify(Message) Intent` — Message is a tgbotapi-free struct
  (Text/Caption, HasPhoto, DocumentName); main.go maps the update into it. Delete is an
  exact-form regex; recap words are substring matches; media beats text.
- AI: `internal/ai.Client` net/http chat-completions with 3x retry on 429/5xx
  (exponential backoff + jitter), 30s per-attempt timeout, usage logged.
  `ParseEntry(EntryInput)` uses json_schema strict response_format; schema built Go-side;
  current date + active categories/currencies + default currency injected per request.
- Entry flow in **new package `internal/bot`** (deviation: prompt said "dispatcher in
  main.go"; a separate package keeps main thin and the flow testable with fakes).
  Validation: amount>0, category/currency in list, date ≤ now+1d; details ignored for
  total; source="text", raw_message preserved. Confirmation reply exactly per spec
  (income line only when income; thanks line localized EN/ID via Indonesian-word regex).
- Tests: router table-driven (incl. the required 5 cases + "delete" inside a sentence),
  auth upsert/drop/normalize, ai against httptest (success, 429 retry, 5xx retry, no
  retry on 400, malformed JSON, schema mismatch, per-attempt timeout), bot flow with
  fake store/AI/sender asserting insert payloads + exact replies for the 5 required
  inputs, no-AI-for-other-intents, validation failures, unauthorized drop.
- Watch out for session 3: delete/recap/photo/pdf intents currently reply
  "coming in the next session" in `bot.Dispatcher.Handle`; the stubs are one-liners to
  replace. `bot.FormatAmount(cur, numericString)` handles IDR dot-thousands vs comma
  thousands for others.
- Not manually verified against a real bot token (none provided).
