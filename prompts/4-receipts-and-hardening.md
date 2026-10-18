# Session 4 — Receipts (photo/PDF) + hardening + /help

Paste the block below into a fresh AI session running in `financio2.0/` (the repo root).

````
You are building Financio 2.0, session 4 of 5: media receipts, hardening, and bot UX.
Read docs/BRIEF.md and status.md first, then survey the repo (sessions 1-3 delivered the
store, bot skeleton, entry flow, delete, and recap).

Carry-over from a 1.0-parity audit (small, do first): extend the ParseEntry system
prompt's date-word examples in `internal/ai` to include "tomorrow"/"besok" (1.0
explicitly supported future dates; validation already allows up to 1 day ahead).

Part A — photo & PDF receipts (1.0 parity):

1. `internal/receipt` with two flows sharing one parser.
2. Photo flow (IntentPhoto): download the largest photo via getFile+download (streaming
   to memory is fine, cap at 10MB), base64 data-URL into a vision message to gpt-4o-mini
   with a json_schema that wraps the session-2 entry schema:
   { "transactions": [ <entry schema> ] }
   System prompt (ported from 1.0): decide A) single receipt -> ONE transaction, line
   items into details; B) statement/list -> one transaction per row with its own date.
   Currency default {DEFAULT_CURRENCY}. Use dates printed on the document when present.
   Pass the photo's caption (if any) as accompanying text; data visible in the image
   takes precedence over the caption (1.0 rule: prioritize the image).
3. PDF flow (IntentPDF): dependency github.com/ledongthuc/pdf — extract text; if empty
   or >30KB, reply "PDF could not be read" (localized) and stop. Feed the text to the
   model with 1.0's rules: keep spending rows (Duitnow QR, Payment, Card Payment, RFID),
   SKIP receive-type rows (Receive, Received, Receive from Wallet) — those are wallet
   top-ups, not income; per-line transactionDate from the document; aggregate details
   into one or many transactions as the photo flow does. Line-item discounts and
   vouchers must appear as detail rows (negative amount or noted in the item name).
   Record the document's transaction type per detail row ("transactionType") when the
   document shows one (Duitnow QR, Card Payment, RFID Payment, ...) — 1.0 kept this for
   auditing.
4. Shared post-processing: media flows inject the same EntryInput context as the text
   flow (active categories, active currencies, today in {DISPLAY_TZ}, default currency).
   For each parsed transaction, run the same validation + insert + reply path as
   session 2's entry flow (reuse it; source="photo"/"pdf"). An empty transactions[] or
   a clearly non-financial document gets a localized "No transactions found in this
   document." reply. When >1 transaction is inserted, send one compact reply instead
   of N confirmations:
       ✅ 3 transactions saved
       🆔 45: Giant RM 16.78 (Groceries, 2026-10-14)
       🆔 46: Tolls RM 2.30 (Transportation, 2026-10-15)
       💰 Total added: MYR 19.08
5. Tests: parser fixtures for single-receipt and multi-row statement JSON; aggregation of
   per-transaction replies; PDF text-size guard; empty transactions[] -> localized
   no-transactions reply; a PDF fixture where a discount row and a "Receive from Wallet"
   row must produce respectively a detail entry and no transaction. The Telegram
   download and OpenAI call are behind interfaces and faked in tests.

Part B — hardening and UX:

6. `/start` and `/help` commands: a compact bilingual usage card:
   • Log: `10rm nasi lemak` / `nasi lemak 10rm yesterday` / `gaji 6500`
   • Delete: `hapus 123` / `delete 123`
   • Recap: `recap nov 2025` / `recap this week meals` / `rekap bulan ini`
   • Send a receipt photo or a PDF statement
7. Error containment: one panic in any handler must not kill the poll loop (recover,
   log, continue). Every outbound AI/Telegram call already has timeouts; add an overall
   per-message processing timeout of 90s.
8. Resilience: Telegram 409 Conflict (another poller running) -> log loudly and exit(1)
   with a clear message (single-replica contract per BRIEF). Postgres outage -> log,
   sleep with backoff, keep polling (messages are not acked until processed
   successfully).
9. Observability: slog fields on every message (update_id, user_id, intent, latency_ms,
   outcome); a startup log line with build info injected via -ldflags.
10. Concurrency: at most 4 messages processed concurrently (bounded worker pool) while
    preserving per-chat ordering — simplest correct shape: a mutex per chat id.
11. README.md: what it is, config table, local run instructions.

Acceptance criteria:
- `go build ./... && go vet ./... && go test ./...` pass.
- A photo of a receipt and a bank-statement PDF both produce correct multi-transaction
  inserts with per-row dates; receive-type PDF rows are skipped.
- Killing Postgres mid-run does not crash the bot and it recovers after Postgres
  returns; a panicking handler is logged and the bot keeps answering.
- Tests cover the /help text and panic recovery.
- Commit; update status.md (session 4 -> Done).
````
