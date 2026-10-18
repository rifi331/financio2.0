# Session 6 (optional) — Import 1.0 data

Paste the block below into a fresh AI session running in `financio2.0/` (the repo root).
Optional: run any time after session 5, only if the old n8n-era data should be kept.

````
Financio 2.0, optional session: one-off import from the 1.0 n8n-era database. Read
docs/BRIEF.md and status.md first.

1. `cmd/import-v1/main.go`: reads the old `transaction_log` (columns: id, user_name,
   item, total_amount, currency, date, description, category_id) from a second DSN env
   V1_DATABASE_URL and copies into the new schema:
   - map distinct old user_name values -> app_user rows with synthetic negative telegram
     ids (e.g. -(rownum)) only when the name matches ALLOWED_USERS; the real telegram id
     binds automatically on that user's next message (extend store.EnsureUser to upsert
     by display_name too when the existing row has a synthetic negative id).
   - map old currency strings (RM->MYR, Rp/IDR->IDR, USD, SGD; unknown ->
     DEFAULT_CURRENCY and log a warning).
   - direction: all 'expense'; keep old description JSONB as-is; source 'text';
     raw_message null.
2. Idempotent: track imported old ids in a v1_imported_ids table and skip them on
   re-run.
3. Dry-run mode printing the mapping; `--apply` flag to write.

Acceptance criteria: row counts match; a recap over an imported month matches the
numbers the 1.0 n8n recap produced for the same window; commit; update status.md
(session 6 -> Done).
````
