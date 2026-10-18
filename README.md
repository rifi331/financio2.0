# Financio 2.0 — Build Prompt Series

Phased, copy-paste prompts to build Financio 2.0: a single lightweight Go binary running a
private Telegram expense/income tracker backed by Postgres and GPT-4o-mini, deployed to the
k3s cluster on the TrueNAS server. Replaces the n8n "Financio postgres" automation in
`G:\projects\financio\`.

## File map

| File | Purpose |
|---|---|
| `docs/BRIEF.md` | Shared context every prompt reads: stack, schema, config, runtime contract |
| `prompts/1-foundation.md` | Session 1 — bootstrap, config, DB schema, migrations, store |
| `prompts/2-bot-and-entry.md` | Session 2 — Telegram polling, allowlist auth, router, AI text-entry flow |
| `prompts/3-delete-and-recap.md` | Session 3 — delete + recap flows |
| `prompts/4-receipts-and-hardening.md` | Session 4 — photo/PDF receipts, /help, resilience |
| `prompts/5-deploy.md` | Session 5 — Docker image + TrueNAS k3s manifest |
| `prompts/6-optional-import-v1.md` | Optional — one-off import of 1.0 data |
| `verification.md` | End-to-end checklist to run after session 5 |
| `status.md` | Progress tracker — update after every session |

## How to use

1. **One prompt file = one fresh AI session.** Open a new agent session (e.g. ZCode) with
   `financio2.0/` as the working directory — this folder is the Go repo root.
2. Paste the entire fenced block from the prompt file into the session.
3. Let the session run to completion: build + tests green, one or more commits made.
4. Update `status.md` (mark the session Done, add date and notes).
5. Repeat with the next prompt file. Run `verification.md` after session 5.

Sessions must be run in order 1→5; prompt 6 is optional and can run any time after 5.

> ⚠️ Security note: the old `G:\projects\financio.yaml` contains plaintext, live-looking
> TELEGRAM_BOT_TOKEN and OPENAI_API_KEY values. **Rotate both before deploying 2.0.**
> The prompts in this folder only ever use placeholders.
