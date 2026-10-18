.PHONY: build test lint run clean image

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

IMAGE      ?= financio:2.0.0
GHCR_IMAGE ?= ghcr.io/rifi331/financio2.0:2.0.0

LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

build:
	go build -ldflags "$(LDFLAGS)" -o dist/financio ./cmd/financio

test:
	go test ./...

lint:
	go vet ./...

run:
	go run ./cmd/financio

image:
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg DATE=$(DATE) \
		-t $(IMAGE) -t $(GHCR_IMAGE) .

clean:
	rm -rf dist
