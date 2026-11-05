# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Copia CLI is a command-line interface for [Copia](https://copia.io) — the source control platform for industrial automation. Modeled after GitHub CLI (`gh`), built on the Gitea-compatible REST API. Phase 1-3 complete with 35+ subcommands across 11 command groups plus `browse`.

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

**Horde sandbox:** `.devcontainer/compose.yaml` (service `dev`, image `.devcontainer/Dockerfile`) is the environment `horde-dev up` starts on the sandbox; run commands there with `horde-dev run go test ./...`, `horde-dev run golangci-lint run ./...`. It is separate from the local workflow above, which it does not change. Its `FROM golang:<version>-alpine` line, the root `Dockerfile`'s and the `go` directive of `go.mod` name one Go version (the image sets `GOTOOLCHAIN=local`), and `test/workflows` fails when they differ, so a Go bump changes all three together.

**CI coverage:** `ci.yml` runs the unit tests through gotestsum and uploads `coverage.out` and `unit.junit.xml` (Test Analytics) to Codecov with GitHub OIDC (`use_oidc: true`, `id-token: write` on the `test` job), no token. PRs get `codecov/patch` (80% of changed lines) and `codecov/project` (at most 1% drop); never soften them in `codecov.yml`. The `copr-install` job runs the hash-pinned `Install copr-cli` step of `copr.yml` (release-only) on every PR, without secrets; `test/workflows` keeps both run blocks identical, so change them together. The `snap-pack` job (`ubuntu-24.04`, `contents: read`, no secrets, `persist-credentials: false`) packs the snap on every PR with snapcraft from the `9.x/stable` track (a pin bumped by hand, Dependabot does not track it) and checks that `meta/snap.yaml` has `version` equal to `git describe --tags --always` without the `v` and `architectures` `[amd64]`; `snap/snapcraft.yaml` sets the version with `craftctl set version` in `override-build`, never `version: git` (the release tags are lightweight), and builds amd64 only (`test/workflows` enforces both). The release jobs `aur.yml`, `copr.yml` and the `build` job of `snap.yml` run the version gate (`id: version`, `script/check-release-version.sh`) right after a checkout with `persist-credentials: false`. `snap.yml` (`ubuntu-24.04`) uses no `snapcore/*` or `canonical/action-*` action, also not as a `docker://` image (none may appear in any workflow or local action), and has two jobs. `build` holds no secret: its `Install snapcraft` and `Pack snap` run blocks are identical to those of `snap-pack`, so change them together, it checks the `name` (`copia-cli`), `version` (the tag's) and `architectures` (`[amd64]`) of the packed `meta/snap.yaml` and hands the snap on with `actions/upload-artifact` and its sha256 as a job output. `upload` (`needs: build`, `permissions: {}`) runs no repository code (no checkout): it downloads the artifact, checks it with `sha256sum -c` and `meta/snap.yaml` again, then runs exactly `snapcraft upload --release stable "$SNAP_FILE"` on the checked file with `SNAPCRAFT_STORE_CREDENTIALS` on that step only. No job or step of `snap.yml` sets `if:` or `continue-on-error` (`test/workflows` enforces all of it). A job that holds a secret or a write token (any `write` permission other than `id-token`), or that such a job lists in `needs:` at any depth (`docs.yml` `build`, whose output `deploy` publishes), sets `cache: false` on `actions/setup-go`, so it never builds from the cache `test` saves; `test` keeps its cache, no privileged job needs it (`integration` runs in parallel), and `test/workflows` enforces the rule. The `integration` job of `ci.yml` and `integration-scheduled.yml` runs `Integration tests` without `-v` (a passing test's `t.Logf` would print the test account's private data in the public log); only the `Copia version` step, same `env`, runs `TestAPI_Version` with `-v`, and `test/workflows` enforces it. `test` runs only pinned tools: golangci-lint `version:` equals `GOLANGCI_LINT_VERSION` in `.devcontainer/Dockerfile`, govulncheck runs at a fixed `@version` in `ci.yml` and `govulncheck.yml`, both Codecov steps pin the Codecov CLI `version:` (`v11.3.2`), all bumped by hand (Dependabot does not track them), and no workflow `run:` block may use `@latest`. The `release` job of `release.yml` runs goreleaser at a fixed `version:` (`v2.18.3`), also bumped by hand. Every `codecov/codecov-action` and `goreleaser/goreleaser-action` step, and any other step that sets `with.version`, needs a `version:` matching `^v\d+\.\d+\.\d+$`, no step may set `with.version-file`, input names match regardless of case, and no `codecov/codecov-action` step may set `binary`, or `skip_validation` or `use_pypi` to anything but `false`, since each skips the CLI's signature and checksum check (`test/workflows` enforces it in every workflow and local action). Every `github/codeql-action` step of every workflow and local action uses one SHA and version comment, `upload-sarif` in `govulncheck.yml` included (`test/workflows` enforces it), and Dependabot bumps them as one grouped PR (`codeql-action` group in `.github/dependabot.yml`, read from `main`, so until a release a split bump is moved by hand in one PR). See `CONTRIBUTING.md`.

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
- **Same-origin redirects:** every command builds its HTTP client with `cmdutil.NewHTTPClient()`, which refuses a redirect that changes scheme, host or port; a guard test parses non-test code under `pkg/`, `internal/` and `cmd/` and fails on an `http.Client` literal or `new(http.Client)` outside `pkg/cmdutil/httpclient.go`, and on `http.DefaultClient`, `http.Get`, `Head`, `Post` or `PostForm`, also under an aliased `net/http` import.
- **Integration tests print no account data:** the tests under `test/integration/` log nothing but the Copia version line of `TestAPI_Version` and check output only through the byte-count helpers of `test/integration/helpers_test.go`; a guard in `test/workflows` parses them with `go/parser` (no `integration` tag needed) and fails on any other `t.Log`/`t.Logf`, on `t.Error`/`t.Fatal` calls and on testify calls other than `NoError`, `Error`, `ErrorContains`, `Greater`, `True` and `False` with literal messages, with `helpers_test.go` as its single exemption.
- **Auth precedence:** `--token` flag > `COPIA_TOKEN` env var > config file
- **Multi-instance support:** Config supports multiple hosts; active instance resolved by `--host` flag > `COPIA_HOST` env > git remote URL > first config entry
- **Config location:** `~/.config/copia/config.yml` (Windows: `%USERPROFILE%\.config\copia\config.yml`), file permissions `600`

## Implementation Roadmap

Phase 1 (MVP): auth, repo list/view/clone, issue CRUD, pr CRUD, label list/create — **DONE**
Phase 2: release CRUD, repo create/delete/fork, pr review/diff/checkout, issue edit, Homebrew tap — **DONE**
Phase 3: `copia-cli api` escape hatch, search, orgs, notifications, `-R` flag, completion, Jekyll manual, AGPL license — **DONE**
Phase 4: ~~winget~~, ~~COPR~~, ~~AUR~~, ~~Snap~~, OS keyring, aliases, ~~browse~~, status dashboard, ssh-key, pr checks, changelog, collaborators

**Out of scope:** workflow/run, codespace, copilot, project, cache, GUI

## Reference Documentation

- `docs/api-reference.md` — Full Gitea API endpoint mapping with request/response examples
- `docs/authentication.md` — Auth methods, token generation, config file format, multi-instance setup
- `docs/parity.md` — Feature parity tracker against the Gitea 1.26 API (70 implementable: 50 implemented, 12 full, 7 partial, 1 client side; 12 impossible)
