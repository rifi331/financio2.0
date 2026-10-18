# syntax=docker/dockerfile:1

# ---- builder ----
FROM golang:1.25-alpine AS builder

WORKDIR /src

# Cache deps first.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags "-s -w \
      -X main.version=${VERSION} \
      -X main.commit=${COMMIT} \
      -X main.date=${DATE}" \
    -o /out/financio ./cmd/financio

# ---- final ----
# distroless/static-debian12 ships ca-certificates and tzdata (TLS to
# api.telegram.org and time.LoadLocation both work). If a future base drops
# tzdata, add: COPY --from=builder /usr/local/go/lib/time/zoneinfo.zip /zoneinfo.zip
# and set ZONEINFO=/zoneinfo.zip.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /out/financio /financio

# HEALTH_PORT (default 7777); the bot itself is outbound-only long polling.
EXPOSE 7777

USER nonroot:nonroot

# Distroless has no shell/wget; the binary self-probes /healthz.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD ["/financio", "healthcheck"]

ENTRYPOINT ["/financio"]
