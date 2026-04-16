# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Copia CLI is a command-line interface for [Copia](https://copia.io) — the source control platform for industrial automation. Modeled after GitHub CLI (`gh`), built on the Gitea-compatible REST API. Phase 1-3 complete with 35+ subcommands across 11 command groups.

**Owner:** Qubernetic (AGPL-3.0 + Commercial)

## Development Environment

Aurora-DX workstation with Docker Compose + justfile. No local Go installation needed.

```bash
# First-time setup
cp .envrc.example .envrc && direnv allow .
just setup

# Daily workflow
just dev          # Start the Go container
just test         # Run tests (inside container)
just build        # Build binary (inside container)
just lint         # Run linter (inside container)
just down         # Stop containers

# VS Code / Cursor: "Reopen in Container" (uses docker-compose.dev.yaml)
```

**Claude Code runs on the host** — never inside the devcontainer. Always launch from a host terminal.

## Architecture

Follows the `gh` CLI repository structure (`github.com/cli/cli`).

```
copia-cli/
├── cmd/copia-cli/          # Entrypoint
├── internal/
│   ├── build/              # Version injection (ldflags)
│   ├── config/             # Config & auth management
│   └── copiacmd/           # Root command wiring
├── pkg/
│   ├── cmd/                # Command packages (one per command group)
│   ├── cmdutil/            # Shared CLI helpers (factory, flags, JSON)
│   ├── iostreams/          # TTY-aware I/O abstraction
│   ├── api/                # Gitea SDK wrapper
│   └── httpmock/           # HTTP mock for testing
├── docs/                   # Developer documentation
├── justfile                # Command runner (replaces Makefile)
├── Dockerfile              # Go dev image
├── docker-compose.yml      # Base compose config
└── docker-compose.dev.yaml # Dev overlay (source mount + caches)
```

**Command structure:** `copia-cli <command> <subcommand> [flags]` — mirrors `gh` CLI UX.

## API Foundation

- **Base URL:** `https://app.copia.io/api/v1/{endpoint}`
- **Auth:** `Authorization: token <key>` header (Personal Access Token) — **no anonymous access**
- **470 Gitea REST endpoints**, no GraphQL
- **Pagination:** `?page=1&limit=50`, response headers include `x-total-count` and `Link`
- Swagger spec at `https://app.copia.io/swagger.v1.json` (requires browser auth, not API token)
- Copia-specific extensions (PLC binary diff, DeviceLink) may use undocumented endpoints

## Key Design Constraints

- **Auth-first:** Every API call requires authentication. `copia-cli auth login` is mandatory before any operation.
- **Auth precedence:** `--token` flag > `COPIA_TOKEN` env var > config file
- **Multi-instance support:** Config supports multiple hosts; active instance resolved by `--host` flag > `COPIA_HOST` env > git remote URL > first config entry
- **Config location:** `~/.config/copia/config.yml` (Windows: `%USERPROFILE%\.config\copia\config.yml`), file permissions `600`

## Implementation Roadmap

Phase 1 (MVP): auth, repo list/view/clone, issue CRUD, pr CRUD, label list/create — **DONE**
Phase 2: release CRUD, repo create/delete/fork, pr review/diff/checkout, issue edit, Homebrew tap — **DONE**
Phase 3: `copia-cli api` escape hatch, search, orgs, notifications, `-R` flag, completion, Jekyll manual, AGPL license — **DONE**
Phase 4: ~~winget~~, ~~COPR~~, ~~AUR~~, ~~Snap~~, OS keyring, aliases, browse, status dashboard, ssh-key, pr checks, changelog, collaborators

**Out of scope:** workflow/run, codespace, copilot, project, cache, GUI

## Reference Documentation

- `docs/api-reference.md` — Full Gitea API endpoint mapping with request/response examples
- `docs/authentication.md` — Auth methods, token generation, config file format, multi-instance setup
- `docs/parity.md` — Feature parity tracker (48 implementable commands: 42 full, 6 partial, 14 impossible)
