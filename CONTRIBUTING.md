# Contributing

Thank you for your interest in improving Copia CLI.

## Prerequisites

- **Docker** (or Podman) — required for the containerized dev environment
- **[just](https://just.systems/)** — command runner (`brew install just`)
- **[direnv](https://direnv.net/)** — per-directory env vars (`brew install direnv`)
- **VS Code / Cursor** — optional, supports "Reopen in Container"
- **GitHub CLI (`gh`)** — recommended for issue/PR management

## Quick Start

All Go tooling runs inside a Docker container. No local Go installation needed.

```bash
git clone https://github.com/qubernetic/copia-cli.git
cd copia-cli

cp .envrc.example .envrc && direnv allow .
just setup        # Build image + download Go deps
just dev          # Start the dev container
just test         # Run tests
just build        # Build binary
```

**VS Code / Cursor:** "Reopen in Container" uses the same Docker Compose stack.

## Development Workflow

This repo follows a strict Gitflow workflow. Every contribution goes through these steps:

```
1. Open a GitHub Issue describing the change
2. Fork the repo (external contributors) or create a branch directly (maintainers)
3. Create a branch from develop:
   git checkout develop
   git pull origin develop
   git checkout -b <type>/<issue>-<slug>
4. Make atomic commits using Conventional Commits format
5. Run tests:
   just test
6. Push and open a PR targeting develop:
   git push -u origin <type>/<issue>-<slug>
   gh pr create --base develop
7. Verify test plan items and check them off in the PR description
8. After merge, clean up:
   git checkout develop && git pull origin develop
   git fetch --prune && git branch -d <type>/<issue>-<slug>
```

### Branch Types

| Change type | Branch prefix | Example |
|-------------|---------------|---------|
| New feature | `feature/` | `feature/42-add-search` |
| Bug fix | `fix/` | `fix/17-broken-timeout` |
| Documentation | `docs/` | `docs/23-update-readme` |
| Hotfix | `hotfix/` | `hotfix/89-auth-crash` |

### Commit Format

```
<type>(<optional scope>): <description>
```

Use imperative mood, lowercase after colon, no period.

| Type | When to use |
|------|-------------|
| `feat` | New feature or capability |
| `fix` | Bug fix |
| `docs` | Documentation only |
| `chore` | Maintenance, config, dependencies |
| `refactor` | Code restructure without behavior change |
| `test` | Adding or updating tests |

### Testing

Before submitting a PR:

1. **Unit tests:** `just test`
2. **Build:** `just build`
3. **Lint:** `just lint`

Integration tests require a Copia API token (set in `.envrc`) and run separately:

```bash
just integration
```

## Project Structure

```
cmd/copia-cli/      Entrypoint
internal/           Private packages (config, build, root command)
pkg/cmd/            Command packages (one per command group)
  auth/             login, logout, status
  repo/             list, view, clone, create, delete, fork
  issue/            list, create, view, close, comment, edit
  pr/               list, create, view, merge, close, review, diff, checkout
  label/            list, create
  release/          list, create, delete, upload
pkg/cmdutil/        Shared CLI helpers (factory, flags, JSON)
pkg/iostreams/      TTY-aware I/O abstraction
pkg/api/            Gitea SDK wrapper
pkg/httpmock/       HTTP mock for testing
test/integration/   Integration tests against live Copia API
```

## Guidelines

### Invariants (non-negotiable)

- **Atomic logical commits** — one commit = one change
- **Conventional Commits** format with imperative mood
- **Gitflow branch model** — `main`/`develop` protected, PR-only merges
- **Issue-driven workflow** — every branch traces to a GitHub Issue
- **TDD** — write failing tests first, then implement
- **Semantic Versioning** for releases

### Code Style

- Follow `gofmt` and `go vet` conventions
- Keep files focused — one responsibility per file
- Follow existing patterns (Options struct + NewCmd + Run function)
- Add `--json` support to every list/view command

## Reporting Bugs

Please open an issue with:

- The command you ran
- What happened (include error output)
- What you expected to happen
- Your OS and `copia-cli --version` output
