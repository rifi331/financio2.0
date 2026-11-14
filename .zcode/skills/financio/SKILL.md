---
name: financio
description: Use when working on the Financio 2.0 Telegram expense bot in this repo — its commands, architecture, deployment (TrueNAS Custom App + Cloudflare), MacroDroid notification ingestion, debugging recipes, and release procedure. Covers verified third-party integration facts (MacroDroid trigger behavior, OpenAI/ghcr quirks) learned the hard way.
---

# Financio 2.0 — Project Skill

Telegram expense/income tracker: single Go binary, long polling (outbound-only,
no ingress needed for the bot itself), Postgres, OpenAI structured outputs.
**Read docs/BRIEF.md and status.md first** — they are the authoritative spec
and the full build log with per-version deviations.

## Where things live

- `cmd/financio/main.go` — wiring; ingest callback; daily ticker (recurring + alerts)
- `internal/bot/` — dispatcher + one file per feature (edit, budget, group, recurring, alert, param, prompt, detail, options, history, usercache)
- `internal/store/` — pgx store; one file per area (budget, group, prompt, sysparam, recurring)
- `internal/ai/` — OpenAI client; prompts have BOTH compiled defaults (parse.go/receipt.go/recap.go) and DB-stored copies
- `internal/report/` — full-report PDF (go-pdf/fpdf; pie uses FIXED page coords → one chart page per currency, never two currencies on one page)
- `internal/web/` — dashboard + /ingest on HEALTH_PORT 7777, token-gated (DASHBOARD_TOKEN)
- `internal/router/` — all intents + regexes
- `migrations/` — 0001..0016, applied on startup; DB-stored prompts/sysparams override code defaults

## Runtime model (memorize these; they override env vars)

- `prompt` table overrides compiled prompts; `sysparam` table overrides env
  (`openai_model` applies immediately via /param; timeouts on restart).
  -> A prompt/prompt-behavior bug may live in the DB, not the code. Check
  `/prompt show <name>` before editing Go prompts. Migrations 0007/0016
  REWRITE seeded prompts (overwrites user customizations).
- Budgets are **per budget_group** (Family seeded; new users get solo groups).
  MonthCategorySpend/ListBudgets are group-scoped.
- `direction` values: expense | income | **transfer** (between registered
  users; excluded from recap/budgets/report totals; counterparty in
  description JSONB `{"transferTo": ...}`).
- description JSONB shape: 2.0 = BARE ARRAY of details; 1.0 imports =
  {"details":[...]} wrapper. **Always parse via store.ParseDetails** (handles
  both) — hand-rolled unmarshal only handling one shape is a known bug class.
- Recap is expenses-only. Dashboard shows transfers but excludes them from net.

## Commands (bot)

entry text / photo / statement PDF · `edit <id> [amount X] [category C]
[item ..] [date Y] [currency Z]` · `delete|hapus <id>` · `detail|rincian|info <id>`
· `recap <period> [category] [order by category]` · `check duplicate [hard]
<period>` (hard = amount+date, ignores item names) · `full report <period>
[category] [order by category]` · `/budget set|list|delete` · `/group
list|set` · `/recurring add|list|delete` · `/prompt list|show|set|reset` ·
`/param list|show|set` · `/options` (DB-stored catalog) · `/start|/help`

## Debugging recipes

1. **What did the bot receive / what did the AI decide?** `/logs` page or
   docker logs: `message processed` includes `text` (received, truncated
   200ch); `ai output` line shows the parsed fields.
2. **Test /ingest without the phone:** from PowerShell use `curl.exe`
   (NOT `curl`, that's a PS cmdlet) with the plain-text form —
   `curl.exe -X POST https://financio.parifidev.com/ingest -H "X-Token: <token>" -H "Content-Type: text/plain" -d "paid rm 12 test"`
   (JSON bodies from PS get quote-mangled; plain text accepted since v2.10.2).
3. **MacroDroid not firing:** System Log shows trigger fires; magic text for
   notifications is **{not_title} and {notification}** (via the {} picker).
   **Text-content matching is REGEX, not comma-list** — `(?i)paid|payment|rm`
   works; a comma list is matched as one literal string and matches nothing.
4. **"Could not parse that" on every entry:** check `/param list` (model may
   be one the key can't use) and OpenAI credit; then docker logs for 401/429.
5. **Store tests vs real DB:** unit fakes don't execute SQL — window-function
   in WHERE (fixed in v2.8.6) and param bugs (fixed v2.10.3) slipped through.
   For DB-behavior changes run the suite with FINANCIO_TEST_DSN against a
   scratch schema.

## Release procedure

1. Full gate: `go vet ./...`, `gofmt -l .` empty, `go build ./...`,
   `go test ./...` exit 0 (grep `^--- FAIL`, don't trust grep -v pipelines).
2. Bump image tag in deploy/*.yaml + deploy/README.md + status.md + session log.
3. Commit, push main, `git tag vX.Y.Z`, push tag. CI (GitHub Actions) builds
   and pushes ghcr.io/rifi331/financio2.0:X.Y.Z; ~90s to verify Actions green.
4. User updates the tag in TrueNAS app config; migrations run on startup.
5. Deploy manifests live in deploy/; TrueNAS pulls from ghcr (package is
   public; private would need a ghcr PAT docker login on the host).
