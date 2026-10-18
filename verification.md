# Financio 2.0 — End-to-End Verification

Run after session 5 (`prompts/5-deploy.md`) against the deployed bot.

1. Unauthorized Telegram account sends "10rm test" — no reply, one log line.
2. Authorized: "10rm nasi lemak" → saved with id, category Meals, MYR, today (KL time).
3. "nasi lemak, 10rm yesterday" → entry_date is yesterday.
4. "rp50k bakso kemarin" → IDR 50000, 0 decimal places shown, yesterday.
5. "gaji 6500" → direction income, reply includes ↩️ line.
6. "hello" → friendly hint, no DB write.
7. "delete {id}" / "hapus {id}" → deleted; repeat → "Record not found!".
8. "recap oct 2026" and "rekap bulan ini makan" → correct windows, per-currency totals,
   ID/EN labels correct, long recaps split under 4096 chars.
9. Receipt photo and bank PDF → multi-transaction insert with per-row dates; receive-type
   PDF rows skipped.
10. Kill Postgres 30s → bot logs errors, keeps polling, recovers after Postgres returns.
11. `kubectl -n financio logs` shows startup, migrations, one structured line per message.
12. Confirm the secrets from the old `G:\projects\financio.yaml` (Telegram token, OpenAI
    key) were rotated before going live.
