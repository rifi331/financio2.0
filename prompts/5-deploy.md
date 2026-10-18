# Session 5 — Packaging & TrueNAS deployment

Paste the block below into a fresh AI session running in `financio2.0/` (the repo root).

````
You are building Financio 2.0, session 5 of 5: ship it. Read docs/BRIEF.md and
status.md first, then survey the repo (sessions 1-4 delivered a fully working bot).

1. `Dockerfile`, multi-stage:
   - builder: golang:1.24-alpine, CGO_ENABLED=0, -trimpath -ldflags "-s -w -X main.version=..."
   - final: gcr.io/distroless/static-debian12 (includes ca-certs and tzdata) — verify TLS
     to api.telegram.org works; if tzdata is missing, copy zoneinfo from the builder.
   - Non-root user (distroless `nonroot:nonroot`), HEALTH_PORT exposed.
2. `deploy/application-deploy.yaml` mirroring the shape of the 1.0 manifest
   (Namespace financio, ConfigMap, Secret, Deployment) with PLACEHOLDERS ONLY — never
   real secrets:
   - Secret `financio-secrets`: TELEGRAM_BOT_TOKEN: <TELEGRAM_BOT_TOKEN>,
     OPENAI_API_KEY: <OPENAI_API_KEY>, DATABASE_URL: <DATABASE_URL>
   - ConfigMap `financio-config`: OPENAI_MODEL=gpt-4o-mini, DISPLAY_TZ=Asia/Kuala_Lumpur,
     DEFAULT_CURRENCY=MYR, ALLOWED_USERS=<COMMA_SEPARATED_NAMES>, LOG_LEVEL=info
   - Deployment: image ghcr.io/rifi331/financio:2.0.0 (or local tag), replicas: 1 (HARD
     requirement — long polling holds the getUpdates lock; never scale past 1),
     imagePullPolicy IfNotPresent, envFrom secret+configMap, liveness probe on /healthz
     port 7777, resources requests 25m/64Mi limits 250m/192Mi, securityContext
     readOnlyRootFilesystem true, runAsNonRoot true.
   - No Service, no Ingress — the bot is outbound-only (long polling); /healthz is for
     the kubelet only.
3. `deploy/README.md`: two paths to get the image onto TrueNAS k3s — (a) push to ghcr
   and reference it, (b) `docker save financio:2.0.0 | k3s ctr images import -` on the
   host; then `kubectl apply -f deploy/application-deploy.yaml`; how to check logs and
   that schema_migrations ran on startup.
4. Makefile: `image` target tagging consistently (financio:2.0.0).
5. Final sweep: `go vet ./...`, `go test ./... -race`, `gofmt -l` empty, `go build`
   clean; README run section current; status.md session 5 -> Done.

Acceptance criteria:
- Image builds and is < 30MB (document the `docker images` size check in deploy/README).
- Container starts standalone and fails fast with clear logs when required env is
  missing.
- Manifest applies cleanly to a client dry-run (`kubectl apply --dry-run=client`).
- Commit tagged v2.0.0.
````

After this session, run the end-to-end checklist in `verification.md`.
