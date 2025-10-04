GO ?= go
BINARY ?= tester

.PHONY: all bootstrap vendor build test lint tidy run-cli

all: build

bootstrap: tidy vendor
	@echo "Bootstrap complete. Implement dependency checks in later phases."

vendor:
	$(GO) mod vendor

build:
	$(GO) build ./...

lint:
	$(GO) vet ./...

test:
	$(GO) test ./...

tidy:
	$(GO) mod tidy

run-cli:
	$(GO) run ./cmd/tester -- version

