# Session 2 — Bot skeleton + AI text-entry flow

Paste the block below into a fresh AI session running in `financio2.0/` (the repo root).

````
You are building Financio 2.0, session 2 of 5: the Telegram bot skeleton and the core
natural-language entry flow. Read docs/BRIEF.md and status.md first, then survey the
repo (session 1 delivered config, migrations, and the store).

Part A — Telegram polling, allowlist auth, router:

1. Dependency: github.com/go-telegram-bot-api/telegram-bot-api/v5.
2. `internal/telegram`: a thin wrapper — `Client` with `Run(ctx, handler func(Update))`
   doing long polling (`getUpdates`, timeout 60s, `allowed_updates: ["message"]`),
   internal update-offset bookkeeping, and `SendMessage(chatID, text)` + `SendMessages`
   (multi-part). Plain text only (no HTML parse mode) to avoid escaping bugs.
3. `internal/auth`: allowlist middleware.
   - Config provides `[]string AllowedUsers` (display names, e.g. "Rifi Anggriawan").
   - Inbound message: look up `app_user` by telegram ID; if found and active -> pass.
     If not found: if `first_name + " " + last_name` matches an allowed name -> call
     `EnsureUser` (binds the telegram ID permanently) -> pass. Else -> drop silently and
     log at info level (do NOT reply; replying leaks the bot's existence).
   - Export `Authorizer.Authorize(ctx, msg) (User, bool, error)`.
4. `internal/router`: classify an authorized message into an intent:
   - `IntentDelete`: regexp `(?i)^\s*(hapus|delete|del)\s+#?(\d+)\s*$` -> capture id.
   - `IntentRecap`: text contains (case-insensitive) any of: recap, rekap, summary,
     ringkasan.
   - `IntentEntry`: everything else.
   - Photos (largest size present) -> `IntentPhoto`; document with .pdf filename ->
     `IntentPDF`. Media beats text classification.
   - Note: a message that merely contains the word "delete" inside a sentence must NOT
     be IntentDelete — only the exact command form matches.

Part B — AI client and the entry flow:

5. `internal/ai`: minimal OpenAI chat-completions client over net/http (no SDK):
   - `Client.Chat(ctx, req ChatRequest) (string, error)`; retries 3x on 429/5xx with
     exponential backoff + jitter; 30s timeout per attempt; logs token usage.
   - `ParseEntry(ctx, EntryInput) (ParsedEntry, error)` using model from config
     (gpt-4o-mini) with `response_format: {type:"json_schema", strict:true}` and this
     schema (build the json_schema Go-side):
     {
       "is_transaction": boolean,
       "item": string,                    // short label
       "category": string,                // must be one of the provided list
       "direction": "expense"|"income",
       "amount": number,                  // major units, e.g. 10.50
       "currency": string,                // ISO code, must be one of the provided list
       "date": "YYYY-MM-DD",
       "details": [ {"item":string, "amount":number, "qty":integer, "location":string} ]
     }
6. System prompt for ParseEntry (port of 1.0's rules — keep the substance, tighten the
   wording):
   - You are a finance parsing agent for a Telegram expense tracker. Extract item,
     amount, currency, date, category, direction from the user's message.
   - Currency: if not specified use {DEFAULT_CURRENCY}. "rm"/"myr" -> MYR, "rp"/"idr" ->
     IDR, "sgd"/"s$" -> SGD, "$"/"usd" -> USD. Amounts like "10rm" or "rp50k" or "2.5k"
     must be normalized (50k -> 50000).
   - Date: prefer dates stated in the message ("yesterday", "kemarin", "last friday",
     "kemaren lusa", "tomorrow", "besok" — future dates allowed, validation accepts up
     to 1 day ahead); otherwise use the provided current date. Current date (in
     {DISPLAY_TZ}) is injected into the user message by Go, not guessed by the model.
   - Category: choose exactly one from the provided list.
   - Direction: income when the message is about receiving money (gaji, salary, bonus,
     refund, received, diterima, transfer masuk); otherwise expense.
   - Greetings/chit-chat or anything that is not a transaction: is_transaction=false.
   - The active categories and currencies from the DB are injected per request.
7. Entry flow (wire into the dispatcher for IntentEntry): fetch categories+currencies ->
   ParseEntry -> if !is_transaction reply with a short friendly hint (localized "EN"/"ID"
   by a cheap heuristic — contains any of rekap|kemarin|bulan|minggu|gaji|hapus|terima|
   untuk|saya or other Indonesian words -> ID); else validate (amount>0, category in
   list, currency in list, date not more than 1 day in the future; details sum ignored —
   total wins) -> store.InsertTransaction with source="text", raw_message preserved ->
   reply:
       🆔: {id}
       🛍️: {item}
       💵: {symbol} {amount formatted to currency.decimal_places}
       🗓️: {date}
       🔠: {category}
       ↩️: Income (only when direction=income; omit line for expenses)
       Terima kasih, {first name}! / Thanks, {first name}!  (by detected language)
   On validation failure or AI error: reply "Could not parse that — try e.g. '10rm nasi
   lemak' / 'gaji 5000 bulan ini'" and log details.
8. Amount formatting helper: respect currency.decimal_places (IDR -> "Rp 150.000" with
   thousands separator; MYR -> "RM 10.50").
9. Dispatcher wiring in main.go: config -> pgxpool -> Migrate -> Authorizer -> Client.Run
   with the dispatcher. IntentDelete, IntentRecap, IntentPhoto, IntentPDF are handled by
   stub replies ("coming in the next session") — they arrive in sessions 3 and 4.

Acceptance criteria:
- `go build ./... && go vet ./... && go test ./...` pass; router and auth fully
  unit-tested without network.
- Router table-driven tests cover: "Delete 123", "hapus #7", "recap nov 2025", "10rm
  nasi lemak", "rekap bulan ini makan", and a sentence merely containing "delete"
  (must be IntentEntry, not IntentDelete).
- ai client tested against an httptest server: success, 429 retry, malformed JSON,
  schema mismatch.
- Flow test with a fake store asserting the insert payload for: "10rm nasi lemak" (MYR
  10, today), "nasi lemak, 10rm yesterday" (yesterday), "rp50k bakso kemarin" (IDR
  50000, yesterday), "gaji 6500" (income), "hello there" (is_transaction=false). Exact
  reply format asserted.
- Manually (if a bot token is provided): bot answers only the allowlisted Telegram
  account; others are ignored with no reply. Graceful shutdown cancels polling cleanly.
- No AI call happens for delete/recap/media intents.
- Commit; update status.md (session 2 -> Done).
````
