# Deploying Financio 2.0

The bot is a **single replica, outbound-only** process (Telegram long polling holds
the `getUpdates` lock). Never scale past 1, and no Service/Ingress is needed —
`/healthz` on port 7777 is for the Docker/kubelet healthcheck only.

> This dev machine and the TrueNAS host have no Docker daemon. The image is
> therefore built by GitHub Actions (`.github/workflows/docker.yml`): push a tag
> (`git tag v2.0.0 && git push --tags`) and the workflow builds, pushes to
> `ghcr.io/rifi331/financio2.0:<tag>`, and runs the acceptance checks (image size
> < 30 MB via `docker inspect --format '{{.Size}}'`, and fail-fast on missing
> env with a clear `financio exited with error` log line). TrueNAS itself pulls
> the image from ghcr — no docker CLI needed anywhere.

If you ever have a Docker-capable machine, `make image` builds and tags
`financio:2.0.0` + `ghcr.io/rifi331/financio2.0:v2.1.0` locally; check size with
`docker images financio:2.0.0`.

TLS to api.telegram.org works out of the box: `distroless/static-debian12` ships
`ca-certificates` and `tzdata` (so `time.LoadLocation("Asia/Kuala_Lumpur")` works
too). The container's healthcheck uses `financio healthcheck` (the binary probes
its own `/healthz`) because distroless has no shell or wget.

## Deploy path (a) — TrueNAS Custom App via ghcr (preferred)

1. Ensure the image exists: push tag `v2.0.0` and let the GitHub Actions
   workflow build + push `ghcr.io/rifi331/financio2.0:v2.1.0`. (First time: make the
   ghcr package public, or add a TrueNAS pull secret for `ghcr.io`.)
2. TrueNAS UI → **Apps → Discover Apps → Install via YAML**.
3. Paste `deploy/truenas-custom-app.yaml`.
4. Fill the `<PLACEHOLDER>` env values in the UI (`TELEGRAM_BOT_TOKEN`,
   `OPENAI_API_KEY`, `DATABASE_URL`, `ALLOWED_USERS`). Never commit filled values.
5. Start the app; confirm Running and check logs (Apps → financio → workloads →
   logs, or `midclt`/`k3s kubectl -n ix-financio logs` on the host).

Validate the YAML anywhere with docker compose installed (placeholders are fine):

```sh
docker compose -f deploy/truenas-custom-app.yaml config   # validated ✅
```

## Deploy path (b) — kubectl apply on k3s

TrueNAS SCALE runs k3s under the hood, but the cleaner path is a plain k3s
cluster. The manifest in `deploy/application-deploy.yaml` ships with placeholders
only (Namespace, Secret, ConfigMap, Deployment; validated to parse — the
`--dry-run=client` openapi validation needs a live cluster, run it there):

```sh
kubectl apply --dry-run=client -f deploy/application-deploy.yaml   # on a machine with cluster access
kubectl apply -f deploy/application-deploy.yaml
kubectl -n financio create secret generic financio-secrets \
  --from-literal=TELEGRAM_BOT_TOKEN='...' \
  --from-literal=OPENAI_API_KEY='...' \
  --from-literal=DATABASE_URL='postgres://...' \
  --dry-run=client -o yaml | kubectl apply -f -   # beats placeholders via apply-override
```

(Alternatively keep an untracked, gitignored `deploy/application-deploy.local.yaml`
with real values and apply that — it must never be committed.)

## Deploy path (c) — airgapped k3s (no registry)

On any machine with Docker and the image built (`make image`):

```sh
docker save financio:2.0.0 | gzip > financio.tar.gz
# copy to the TrueNAS/k3s host, then:
gunzip -c financio.tar.gz | k3s ctr images import -
```

Use image `financio:2.0.0` (local tag) in the Deployment instead of the ghcr
reference; `imagePullPolicy: IfNotPresent` picks up the imported image.

## Post-deploy checks

You want, in order:

1. `starting financio` with `version=2.0.0` and the redacted config line,
2. a migration line / no migration errors — `schema_migrations` ran (verify:
   `psql "$DATABASE_URL" -c 'select * from schema_migrations order by id'` —
   expect 0001..0003),
3. `telegram polling started`,
4. health passing (container shows healthy / pod not restarting).

Then send `/start` to the bot from an allowlisted Telegram account and work
through `verification.md`.
