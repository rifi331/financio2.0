# Session 3 — Delete + recap flows

Paste the block below into a fresh AI session running in `financio2.0/` (the repo root).

````
You are building Financio 2.0, session 3 of 5: the delete and recap flows. Read
docs/BRIEF.md and status.md first, then survey the repo (sessions 1-2 delivered the
store, bot skeleton, router, and the AI entry flow with its OpenAI json_schema client).

Part A — delete:

1. Wire IntentDelete: take the id captured by the router regexp, call
   `store.DeleteTransactionByID(ctx, id, user.ID)`.
2. Replies (localized by the same EN/ID heuristic from session 2):
   - success:  `🗑️ item deleted for id: {id}\n🛍️: {item}` / ID: `item dengan id {id} sudah dihapus`
   - not found or not owned: `Record not found!` / `Catatan tidak ditemukan!`
     (1.0 leaked "item owner is not matched! <owner name>" — do not repeat that; never
     reveal another user's data).
3. Guard rails: id parsed as int64; anything non-numeric already failed the router
   regexp in session 2.
4. Unit tests with a fake store for all three outcomes + the exact reply strings in both
   languages.

Part B — recap:

CRITICAL difference from 1.0: the AI must NEVER produce SQL. It returns a structured
filter; Go builds a parameterized query via store.ListTransactions.

5. Extend `internal/ai` with `ParseRecapFilter(ctx, text, today time.Time, categories
   []string) (RecapFilter, error)` reusing the json_schema mechanism:
   {
     "is_recap_request": boolean,
     "date_from": "YYYY-MM-DD",
     "date_to":   "YYYY-MM-DD",
     "categories": [string],     // subset of the provided list, empty = all
     "language": "EN"|"ID"
   }
   System prompt substance (ported from 1.0): interpret EN and ID phrases — "recap nov
   2025", "recap sept 2020", "recap for today", "rekap bulan ini", "kemarin", "minggu
   ini" (Mon-Sun), "bulan lalu", "this year", plus multi-category ("Recap May 2026 Meals,
   Drinks"). The current date in {DISPLAY_TZ} is injected by Go; the model must not guess
   dates. Category names are matched case-insensitively against the provided list
   (fuzzy-ish: "makan" may map to "Meals" — instruct the model to map synonyms).
6. Validation in Go: from <= to; if either bound is zero/invalid -> error reply; clamp
   range to at most 5 years; categories intersected with the DB list (drop unknowns).
7. Formatting (`internal/recap`): given []Transaction build the message(s), porting
   1.0's look:
       📅 Summary {date_from} - {date_to}   (ID: 📅 Ringkasan ...)
       - 🗓️ {date}:
         - 🆔: {id}, 🛍️: {item}, 💵: {amount formatted per its currency}
       ...
       💰 Total: MYR 123.40 + IDR 250.000     <- one total PER currency, never mixed
       🔠 by category (only when filter had categories OR >1 category present):
       Meals MYR 45.20 | Groceries MYR 78.20 | ...
   Split into multiple messages at 4000 chars (Telegram hard limit 4096), never cutting
   an item line; re-emit the date header when continuing; total block always in the last
   message. Header words and "Total" localized.
8. Empty result: "No transactions in this period." / "Tidak ada catatan pada periode ini."
9. Tests: filter-parse against httptest fixtures ("recap nov 2025", "rekap bulan ini
   makan", "recap today", garbage -> is_recap_request=false); formatter tests: grouping
   order (date asc, id asc), multi-currency totals, 4000-char split with re-headed
   continuation, empty input; a test asserting no code path concatenates user text into
   SQL.

Acceptance criteria:
- `go build ./... && go vet ./... && go test ./...` pass.
- A user can never delete another user's row; the delete replies match the exact strings.
- Recap over a mixed-currency period shows one total per currency; long recaps split
  under 4096 chars with correct continuation headers.
- Commit; update status.md (session 3 -> Done).
````
