.PHONY: build test lint run clean

build:
	go build -o dist/financio ./cmd/financio

test:
	go test ./...

lint:
	go vet ./...

run:
	go run ./cmd/financio

clean:
	rm -rf dist
