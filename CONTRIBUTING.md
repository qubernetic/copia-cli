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

#### Coverage gate

CI runs the unit tests through [gotestsum](https://github.com/gotestyourself/gotestsum) and uploads two files to Codecov: the coverage profile (`coverage.out`) and the JUnit test report (`unit.junit.xml`) for Codecov Test Analytics, which tracks failing and flaky tests. The upload authenticates with GitHub OIDC (`use_oidc: true`, with `id-token: write` on the `test` job), so no Codecov token is stored in the repository. GitHub gives no OIDC token to workflows on fork or Dependabot PRs, so those PRs get no Codecov upload and no Codecov statuses.

Every PR gets two Codecov statuses. Their targets come from the organisation's Codecov configuration; `codecov.yml` repeats the patch target and must not lower either gate:

- **`codecov/patch`** fails when less than 80% of the PR's new or changed lines are covered.
- **`codecov/project`** fails when the PR lowers total coverage by more than 1% against the base branch.

Code that cannot sensibly be tested may be excluded in `codecov.yml` `ignore:`, each entry with a comment that gives the reason.

The statuses are not required checks in branch protection, so a red status does not block the merge on its own; treat it as a review finding.

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
- **Release tags** - only a repository admin can create a `v*` tag, and a pushed `v*` tag cannot be deleted or moved by anyone, so a wrong tag means a new patch version (see [docs/releasing.md](docs/releasing.md))

### Code Style

- Follow `gofmt` and `go vet` conventions
- Keep files focused — one responsibility per file
- Follow existing patterns (Options struct + NewCmd + Run function)
- Add `--json` support to every list/view command

### Workflows

Every file under `.github/workflows/` follows three rules:

- **Least-privilege token.** The workflow sets `permissions: contents: read` at the top level. A job that needs more widens it in its own `permissions:` block and grants only what its steps use (for example `issues: write` on the job that opens an issue, `security-events: write` on the job that uploads SARIF). A job-level block replaces the top-level one, so repeat `contents: read` there when the job checks out code. Steps that hold secrets and run repository code do not share a job with a widened token.
- **Actions pinned by commit SHA.** Every `uses:` line names the full 40-character commit SHA with the release tag in a trailing comment, for example `actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1`. Resolve the SHA with `gh api repos/<owner>/<action>/commits/<tag> --jq .sha`, which returns the commit even for an annotated tag. Dependabot (`github-actions` ecosystem) updates the SHA and the comment together.
- **No expressions in `run:`.** A `${{ }}` inside a `run:` block is pasted into the script before the shell parses it, so a secret, a step output or a value such as `github.ref_name` becomes shell source. Pass the value through the step's `env:` and reference it as a quoted shell variable (`"$VERSION"`). A value read from outside the repository is checked before it reaches `GITHUB_OUTPUT`: bump-go runs `script/check-go-version.sh` on the version it reads from go.dev; aur and copr run `script/check-release-version.sh` on the version taken from the release tag, and aur reads each checksum from `checksums.txt` with `script/select-checksum.sh`, which takes only the one line naming the exact archive (`copia-cli_<version>_linux_<arch>.tar.gz`), then runs `script/check-sha256.sh` on it. `test/workflows` fails `go test ./...` on any `${{` inside a `run:` block.

## Reporting Bugs

Please open an issue with:

- The command you ran
- What happened (include error output)
- What you expected to happen
- Your OS and `copia-cli --version` output
