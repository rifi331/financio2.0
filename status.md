# Financio 2.0 — Build Status

Update this file after every completed prompt session: set Status to `Done`, fill in the
date, and note anything deviated from the prompt (skipped items, decisions made).

Legend: `Pending` → `In Progress` → `Done` (or `Skipped` with a reason).

| # | Prompt file | Scope | Status | Session date | Notes |
|---|---|---|---|---|---|
| 1 | `prompts/1-foundation.md` | Repo bootstrap, config, DB schema, migrations, store | Done | 2026-10-18 | All items done. Placeholder-package layout in place for sessions 2–4. |
| 2 | `prompts/2-bot-and-entry.md` | Telegram polling, allowlist auth, router, AI text-entry flow | Done | 2026-10-18 | All items done. Dispatcher lives in new `internal/bot` package. |
| 3 | `prompts/3-delete-and-recap.md` | Delete + recap flows | Done | 2026-10-18 | All items done. |
| 4 | `prompts/4-receipts-and-hardening.md` | Photo/PDF receipts, /start & /help, resilience | Done | 2026-10-18 | All items done. See session log below. |
| 5 | `prompts/5-deploy.md` | Docker image, TrueNAS custom-app YAML + k3s manifest, final sweep | Done | 2026-10-18 | All items done; image build moved to GitHub Actions (no Docker on dev machine or TrueNAS). See session log below. |
| 6 | `prompts/6-optional-import-v1.md` | Optional: import 1.0 n8n-era data | Pending | | |
| — | `verification.md` | End-to-end manual verification checklist | Pending | | |

## Deployment TODO — TrueNAS custom app (manual steps, after session 5)

My deploy path of choice: TrueNAS SCALE **Custom App** (Apps → Discover Apps →
Install via YAML) using `deploy/truenas-custom-app.yaml`, with the image
`ghcr.io/rifi331/financio2.0:v2.6.0` pulled from GitHub Container Registry. Neither
this dev machine nor the TrueNAS host runs Docker, so the image is built by
GitHub Actions on tag push (`.github/workflows/docker.yml`) — that workflow also
performs the acceptance checks (size < 30 MB, fail-fast on missing env).

- [ ] Rotate the Telegram bot token and OpenAI key (both exposed in the old `financio.yaml`)
- [ ] Prepare the Postgres database (TrueNAS app) and note the `DATABASE_URL`
- [ ] Push the tag to build/publish the image: `git push origin main --tags`
      (tag `v2.0.0` already exists; verify the Actions run succeeded and the
      package exists at ghcr.io/rifi331/financio2.0:v2.6.0 — make it public once,
      or prepare a ghcr pull secret for TrueNAS)
- [ ] TrueNAS UI → Apps → Discover Apps → Install via YAML → paste `deploy/truenas-custom-app.yaml`
- [ ] Fill the env placeholders in the UI: `TELEGRAM_BOT_TOKEN`, `OPENAI_API_KEY`,
      `DATABASE_URL`, `ALLOWED_USERS` (never commit filled values)
- [ ] Start the app → confirm Running, logs show migrations applied + polling started
- [ ] Send `/start` to the bot from an allowlisted Telegram account
- [ ] Work through `verification.md` end to end

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

### Session 3 — 2026-10-18
- Delete: `bot.handleDelete` — id comes from the router regexp (int64); replies exactly
  `🗑️ item deleted for id: {id}\n🛍️: {item}` (EN) / `🗑️ item dengan id {id} sudah dihapus\n🛍️: {item}`
  (ID); not-found and not-owned both reply `Record not found!` / `Catatan tidak ditemukan!`
  (no owner leakage).
- Recap: `ai.ParseRecapFilter` (json_schema strict, RecapFilter struct: is_recap_request,
  date_from, date_to, categories[], language EN|ID; today date injected by Go; synonym
  mapping instructed). Go validates: from<=to, both parse, range clamped to 5y
  (`parseAndValidateRange`); categories intersected case-insensitively with DB list;
  query via typed `store.TxnFilter` only — model output never becomes SQL text.
- Formatter in `internal/recap`: date-grouped lines, per-currency totals
  (`MYR 30.00 + IDR 150.000`), category breakdown only when filter had categories or
  >1 category present, EN/ID headers, 4000-char split that never cuts a line and
  re-emits the header; totals always in the last message. Empty period short-circuits
  in the dispatcher with the localized empty message.
- `FormatAmount` moved from bot to recap (bot re-exports it as a var so callers are
  unchanged); recap also has `FormatAmountCode` for totals (ISO-code prefix).
- Tests: delete outcomes (success EN/ID, not found, not-owned -> same generic reply and
  row survives) with exact reply strings; recap flow (filter intersect, typed filter
  assertion, empty, invalid range, is_recap_request=false); formatter (grouping,
  multi-currency totals, single-category omits breakdown, ID localization, 120-row
  split with headers/limits, empty); ai filter-parse fixtures (nov 2025, rekap bulan ini
  makan, recap today, garbage, malformed); a guard test that the recap package never
  imports the pgx driver or builds SQL strings.

### Session 4 — 2026-10-18
- Receipts: `ai.ParsePhotoReceipt` (vision message, base64 data-URL image, json_schema
  wrapping session-2's entry schema as `{transactions:[...]}`) and `ai.ParseTextReceipt`
  (PDF text, 1.0 statement rules: keep Duitnow QR/Payment/Card Payment/RFID rows, skip
  receive-type wallet top-ups) share one internal parser (`parseReceipt`).
- `internal/receipt`: `ExtractPDFText` (ledongthuc/pdf), 30KB text guard, localized
  "PDF could not be read" reply, compact multi-transaction confirmation
  ("✅ N transactions saved" + per-row lines + per-currency total). Flow handlers
  live in `bot.Dispatcher` (deviation from "flows in internal/receipt") so both media
  flows reuse the single `insertParsed` validation+insert path with source
  "photo"/"pdf" — the BRIEF's one-insert-path rule won.
- Telegram: `Download` (getFile + download, 10MB cap); polling rewritten to a manual
  `GetUpdates` loop (library's GetUpdatesChan panics on error): 409 Conflict returns
  `ErrConflict` -> main exits(1) loudly (single replica); other getUpdates errors
  back off exponentially (1s..30s) and keep polling. Postgres outages surface as
  per-message error replies; the loop survives.
- Hardening/UX: `/start` + `/help` bilingual card (router IntentHelp); panic recovery
  in `Dispatcher.Handle` (logged with stack, bot keeps answering); 90s per-message
  timeout; `bot.Pool` = max 4 concurrent messages with strict per-chat ordering
  (serial worker per chat id + global semaphore); per-message slog fields
  (update_id, user_id, intent, latency_ms, outcome); startup log line with
  version/commit/date injected via -ldflags (Makefile build updated).
- Tests: photo flow (multi-row aggregate reply exact, per-row dates, source/raw),
  single-receipt confirmation, PDF 40KB guard + localized unreadable (EN/ID), small
  valid PDF end-to-end (via new `internal/receipt/testpdf` minimal PDF builder),
  download failure reply, help text, panic recovery keeps answering, pool ordering;
  router /start|/help classification; ai vision request shape + prompt-rule fixtures;
  receipt formatter incl. multi-currency totals.
- Not manually verified against a real bot token / real OpenAI calls (none provided).

### Session 5 — 2026-10-18
- `Dockerfile`: multi-stage golang:1.24-alpine → distroless/static-debian12:nonroot,
  CGO_ENABLED=0, -trimpath -ldflags "-s -w" + version/commit/date build args,
  nonroot user, EXPOSE 7777. Distroless static ships ca-certs + tzdata, so TLS to
  api.telegram.org and time.LoadLocation work unmodified (noted the zoneinfo
  fallback in a comment).
- Distroless has no shell/wget, so the container healthcheck can't use wget:
  added a `financio healthcheck` subcommand to main.go that self-probes
  /healthz (HEALTH_PORT-aware). Dockerfile HEALTHCHECK and the compose
  healthcheck both use it.
- `deploy/application-deploy.yaml`: Namespace/Secret/ConfigMap/Deployment with
  placeholders only; replicas 1 + Recreate strategy (long polling), envFrom,
  liveness on /healthz:7777, 25m/64Mi → 250m/192Mi, readOnlyRootFilesystem,
  runAsNonRoot, no Service/Ingress. `deploy/application-deploy.local.yaml`
  gitignored.
- `deploy/truenas-custom-app.yaml`: compose-style Custom App def (single service,
  restart unless-stopped, ghcr image, placeholders, header comment with the
  paste-in location). Validated with `docker compose ... config` ✅.
- `deploy/README.md`: three deploy paths + push-to-ghcr + post-deploy checks.
  Deviation: **no Docker on the dev machine or the TrueNAS host**, so the image
  build moved to GitHub Actions (`.github/workflows/docker.yml`): builds and
  pushes on tag push, and runs the acceptance checks itself (size < 30 MB,
  fail-fast on missing env). `kubectl --dry-run=client` needs a live cluster
  (no cluster configured here); the manifest was validated by full YAML parse
  of all four documents instead — run the dry-run on a machine with cluster
  access before applying.
- Makefile: `image` target (tags financio:2.0.0 + ghcr.io/rifi331/financio2.0:v2.6.0).
- Final sweep: `go vet` clean, `go test ./...` all ok (`-race` not runnable on
  this host — needs cgo/gcc), `gofmt -l` empty after formatting two test files,
  `go build` clean. README run section updated with a Deploy pointer.

### Hotfix v2.0.1 — 2026-10-18 (post-deploy)
- Crash on first real message: some Telegram updates carry `Message.From == nil`
  (channel auto-forwards, anonymous group admins) and the poll callback
  dereferenced it → panic-restart loop. Guard added in main.go (skip + warn).
  Found live on TrueNAS; image re-published as v2.0.1.

### Session 6 (ad-hoc) — 2026-10-18: web dashboard (v2.1.0)
- Read-only dashboard on the existing HEALTH_PORT (`internal/web`): `/` transactions
  with month/category/direction filters + per-currency net totals (household-wide,
  via new `store.ListDashboardTransactions`), `/logs` + `/api/logs` tail of an
  in-memory 500-line slog ring, `/login` token gate (cookie, sha256-verged,
  constant-time compare; `?key=` bookmark also works). `/healthz` unchanged and
  always open. Disabled (503) unless DASHBOARD_TOKEN is set.
- Design agreed with user: URL-token auth, read-only + filters, no delete from UI.

### Session 7 (ad-hoc) — 2026-10-18: duplicate check + full PDF report (v2.2.0/v2.3.0)
- v2.1.4: recap is spending-only (TxnFilter.Direction).
- v2.2.0: `check duplicate <month>` / `cek duplikat <bulan>` — store
  ListDuplicateGroups (GROUP BY item+amount+currency+date HAVING count>1),
  reply lists groups with ids ready for delete.
- v2.3.0: `full report <range> [category]` / `laporan lengkap` — PDF via pure-Go
  go-pdf/fpdf (distroless-compatible): summary page, expense pie per currency
  (drawn natively), expense + income tables, links/outline for navigation.
  telegram.Client.SendDocument added. Deviation from BRIEF stack: one new dep
  (go-pdf/fpdf); justified — PDF generation needs it and it's pure Go.

### Session 8 (ad-hoc) — 2026-10-18: DB-stored prompts (v2.4.0)
- All 4 AI system prompts now live in a `prompt` table (migration 0005 seeds
  defaults with {{categories}}/{{currencies}}/{{default_currency}} placeholders);
  Go fills placeholders per request and falls back to compiled-in defaults when
  a row is missing/malformed — a bad edit can never take the bot down.
- New `/prompt` admin command (allowlist-gated): list / show / set / reset.
  Changes apply from the next message, no redeploy.
- v2.3.1 (folded in): period-filter prompt accepts duplicate-check and
  full-report phrasing.

### Session 9 (ad-hoc) — 2026-10-18: sysparam table + timeout fix + model default (v2.5.0)
- Fix: bank-statement PDF parses timed out (30s/attempt x3); per-attempt
  timeout now 80s (ai.Client.AttemptTimeout), per-message 300s.
- New `sysparam` table (migration 0006): openai_model, ai_attempt_timeout_seconds,
  message_timeout_seconds. Loaded at startup (DB > env > default); editable via
  new `/param list|show|set` command (model applies immediately, timeouts on
  restart). Secrets never live in sysparam.
- Default OPENAI_MODEL changed gpt-4o-mini -> gpt-6-luna everywhere (config,
  ai client, migration seed, deploy manifests, BRIEF).

### v2.5.1 — 2026-10-18: report sort option
- "full report august order by category" (also "group by category" /
  "urutkan berdasarkan kategori") now renders expense/income tables grouped
  under category headings, rows by date within each. Default remains one
  date-ordered table. RecapFilter gained order_by (date|category).

### v2.5.2 — 2026-10-18: order-by for recap too
- "recap october order by category" (and any period request) now renders
  grouped under category headings via RecapFilter.order_by; default stays
  date-ordered. Migration 0007 refreshes the seeded recap_filter prompt
  (overwrites customizations made to it before v2.5.2).

### v2.6.0 — 2026-10-18: report polish
- Fix mojibake in PDF text (em dash/middle dot/ellipsis -> ASCII; core
  Helvetica is Latin-1 only).
- "order by category" tables now show a per-category subtotal (per currency).
- New horizontal bar chart per currency next to the pie.
- New "Insights - top 3 expenses" block per currency on the chart page.
