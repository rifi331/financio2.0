# Session 1 — Foundation: bootstrap, config, database layer

Paste the block below into a fresh AI session running in `financio2.0/` (the repo root).

````
You are building Financio 2.0, session 1 of 5: foundation — repo bootstrap, config, and
the full database layer. Read docs/BRIEF.md (authoritative shared context) and status.md
first.

Part A — bootstrap:

1. `go mod init github.com/rifi331/financio` (Go 1.24+).
2. Package layout, empty but compiling:
   - `cmd/financio/main.go`
   - `internal/config/`, `internal/telegram/`, `internal/auth/`, `internal/router/`,
     `internal/ai/`, `internal/store/`, `internal/recap/`, `internal/receipt/`
3. `internal/config`: load the env vars listed in docs/BRIEF.md ("Config" section) using
   `os.Getenv` with defaults; fail fast with a clear message when a required var is
   missing. Add a `String()` redaction method that never prints tokens/DSN passwords.
4. `main.go`: init slog (JSON handler, level from LOG_LEVEL, default info), load config,
   log the redacted config, start a `net/http` server on HEALTH_PORT serving `/healthz`
   -> 200 "ok", shut down gracefully on SIGINT/SIGTERM. No DB or Telegram yet.
5. `.gitignore` (binary, .env, *.env, /dist, deploy/*.local.yaml), `Makefile` with
   targets: `build`, `test`, `lint`, `run`, `check-secrets`. `git init` + initial commit.
6. Secret hygiene, enforcing docs/BRIEF.md "Secrets policy": commit a `.githooks/pre-commit`
   hook that scans staged content for credential patterns and blocks the commit on a
   match — Telegram bot token `\b\d{8,10}:[A-Za-z0-9_-]{35}\b`, OpenAI key
   `sk-[A-Za-z0-9_-]{20,}`, DSN with password `postgres(ql)?://\S+:\S+@`. Activate it via
   `git config core.hooksPath .githooks` and document that step in README.md. The
   Makefile `check-secrets` target runs the same scan over the working tree.
7. Part A is stdlib only — no frameworks, no dependencies yet.

Part B — database:

7. Dependency: github.com/jackc/pgx/v5 (with pgxpool).
8. Migrations runner, hand-rolled (no external migration tool):
   - `migrations/0001_init.sql` ... as plain .sql files, embedded with `embed.FS`; a
     `store.Migrate(ctx, pool)` that applies pending files in name order, tracking
     applied versions in a `schema_migrations(version TEXT PRIMARY KEY, applied_at)`
     table, one transaction per file.
   - 0001: the four tables plus the index exactly as in docs/BRIEF.md ("Schema").
   - 0002: seed currencies MYR(RM,2), IDR(Rp,0), USD($,2), SGD(S$,2) with sort_order 1..4.
   - 0003: seed categories: Groceries, Medical, Utilities, Housing, Meals, Entertainment,
     Living Expenses, Education, Public Transportation, Others.
   - app_user rows are created lazily at runtime, never seeded.
9. `internal/store` repository over pgxpool with a `Store` struct and these methods
   (all context-aware, all parameterized, no fmt.Sprintf SQL):
   - `EnsureUser(ctx, telegramID int64, displayName string) (User, error)` — upsert on id,
     update display_name if changed.
   - `GetUserByTelegramID(ctx, telegramID) (User, bool, error)`
   - `ListActiveCategories(ctx) ([]string, error)`
   - `ListActiveCurrencies(ctx) ([]Currency, error)`
   - `InsertTransaction(ctx, txn NewTransaction) (Transaction, error)` — resolves
     category_id by name (fail with a typed error if unknown), returns the stored row
     including category name (single INSERT .. SELECT .. RETURNING, like 1.0's CTE).
   - `DeleteTransactionByID(ctx, id int64, userID int64) (Transaction, bool, error)` —
     `DELETE ... WHERE id=$1 AND user_id=$2 RETURNING *`; false when no row (covers both
     "not found" and "owned by someone else"; report one generic message, see session 3).
   - `ListTransactions(ctx, f TxnFilter) ([]Transaction, error)` where TxnFilter is
     `{UserID int64, From, To time.Time, Categories []string}` — dynamic WHERE built from
     fixed clauses with placeholders (categories via `= ANY($n)`), ordered by
     entry_date, id.
10. Store tests: gate behind env `FINANCIO_TEST_DSN`; if unset, `t.Skip`. Cover EnsureUser
    upsert, Insert with unknown category error, Delete ownership semantics, List filters
    (date range, category ANY, both).

Acceptance criteria:
- `go build ./...`, `go vet ./...`, `go test ./...` pass (skips allowed without DSN).
- `make run` starts, logs config safely, serves /healthz, exits cleanly on Ctrl+C.
- A unit test asserts config fails fast when TELEGRAM_BOT_TOKEN is unset.
- The secret scanner demonstrably blocks: a test feeds it content containing a fake
  Telegram-format token and asserts detection (use obviously fake values).
- With a real DSN: migrations are idempotent (run twice -> no error), seeds present.
- No SQL string concatenation anywhere (grep for "Sprintf" near SQL must fail).
- Commit; update status.md (session 1 -> Done, add date and notes).
````
