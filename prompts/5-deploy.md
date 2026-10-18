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
   - Real values are substituted only at apply time (e.g. `kubectl -n financio create
     secret generic financio-secrets --from-literal=TELEGRAM_BOT_TOKEN=...`) or via an
     untracked `deploy/application-deploy.local.yaml` (gitignored). A manifest with real
     secrets filled in must never be committed — per BRIEF "Secrets policy" and the
     session-1 pre-commit hook.
3. `deploy/truenas-custom-app.yaml`: a TrueNAS SCALE **Custom App** definition
   (docker-compose style, for Electric Eel and later) — this is the user's preferred
   deploy path:
   - `services.financio`: image `ghcr.io/rifi331/financio:2.0.0` (pulled from GitHub
     Container Registry), `restart: unless-stopped`, single service only (long polling —
     never scale past 1).
   - Environment entries for every var in BRIEF "Config" with `<PLACEHOLDER>` values the
     user fills in the TrueNAS UI — never commit filled values (BRIEF "Secrets policy").
   - Healthcheck hitting `http://localhost:7777/healthz` on the container port.
   - A header comment with the paste-in location: TrueNAS UI → Apps → Discover Apps →
     Install via YAML.
4. `deploy/README.md`: three deploy paths — (a) TrueNAS Custom App via
   `truenas-custom-app.yaml` with the ghcr image (preferred), (b) kubectl apply of the
   k3s manifest, (c) `docker save financio:2.0.0 | k3s ctr images import -` on the host
   for airgapped clusters; plus how to push the image to ghcr (docker login ghcr.io with
   a GitHub PAT, buildx, push) and how to check logs and that schema_migrations ran.
5. Makefile: `image` target tagging consistently (financio:2.0.0).
6. Final sweep: `go vet ./...`, `go test ./... -race`, `gofmt -l` empty, `go build`
   clean; README run section current. Update status.md twice: session 5 -> Done, and
   refresh the "Deployment TODO — TrueNAS custom app" checklist there so image tag,
   file names, and steps match what was actually built (the user reads and works
   through that checklist manually).

Acceptance criteria:
- Image builds and is < 30MB (document the `docker images` size check in deploy/README).
- Container starts standalone and fails fast with clear logs when required env is
  missing.
- Manifest applies cleanly to a client dry-run (`kubectl apply --dry-run=client`).
- `docker compose -f deploy/truenas-custom-app.yaml config` validates (with placeholder
  values), references the ghcr image, and contains no real secrets.
- status.md shows session 5 Done and an up-to-date "Deployment TODO — TrueNAS custom
  app" checklist.
- Commit tagged v2.0.0.
````

After this session, run the end-to-end checklist in `verification.md`.
