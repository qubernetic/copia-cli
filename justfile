set shell := ["bash", "-euo", "pipefail", "-c"]

bin         := "copia-cli"
version     := env("VERSION", "DEV")
date        := `date -u +%Y-%m-%d`
ldflags     := "-s -w -X github.com/qubernetic/copia-cli/internal/build.Version=" + version + " -X github.com/qubernetic/copia-cli/internal/build.Date=" + date
compose_dev := "docker compose -f docker-compose.yml -f docker-compose.dev.yaml"

# List all recipes
default:
    @just --list --unsorted

# ─── Lifecycle ────────

# First-time setup: build image + download deps
setup:
    {{ compose_dev }} build
    {{ compose_dev }} run --rm go go mod download

# Start dev container (detached)
dev:
    {{ compose_dev }} up -d

# Stop containers (keep volumes)
down:
    {{ compose_dev }} down

# Nuclear: remove containers + volumes
clean:
    {{ compose_dev }} down -v
    rm -rf bin/ dist/

# Rebuild image after Dockerfile changes
rebuild:
    {{ compose_dev }} build --no-cache

# Shell into the go container
shell:
    {{ compose_dev }} exec go sh

# ─── Build ────────────

# Build the CLI binary
build:
    {{ compose_dev }} exec go go build -ldflags "{{ ldflags }}" -o bin/{{ bin }} ./cmd/copia-cli

# ─── Checks ───────────

# Run unit tests
test:
    {{ compose_dev }} exec go go test ./...

# Run integration tests (requires COPIA_TOKEN)
integration:
    {{ compose_dev }} exec go go test -tags=integration ./...

# Run acceptance tests
acceptance:
    {{ compose_dev }} exec go go test -tags=acceptance ./acceptance/...

# Run linter
lint:
    {{ compose_dev }} exec go golangci-lint run ./...

# Format code
fmt:
    {{ compose_dev }} exec go gofmt -w .

# ─── Docs ─────────────

# Generate CLI manual pages
docs:
    {{ compose_dev }} exec go go run script/gen-docs.go

# Serve docs site locally
docs-serve: docs
    cd docs/site && uv run -q --with mkdocs-material==9.7.6 mkdocs serve -a 127.0.0.1:8021 --watch-theme

# Build the docs site (strict)
docs-build:
    cd docs/site && uv run -q --with mkdocs-material==9.7.6 mkdocs build --strict

# Clean docs build artifacts
docs-clean:
    rm -rf docs/site/site docs/site/commands-nav.json docs/site/docs/manual/copia-cli*.md

# ─── Release ──────────

# Create a local snapshot release
snapshot:
    {{ compose_dev }} exec go goreleaser release --snapshot --clean
