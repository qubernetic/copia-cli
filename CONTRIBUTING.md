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

Every file under `.github/workflows/` (and, for the second and third rules, every local action a workflow uses) follows five rules:

- **Least-privilege token.** The workflow sets `permissions: contents: read` at the top level. A job that needs more widens it in its own `permissions:` block and grants only what its steps use (for example `issues: write` on the job that opens an issue, `security-events: write` on the job that uploads SARIF). A job-level block replaces the top-level one, so repeat `contents: read` there when the job checks out code. Steps that hold secrets and run repository code do not share a job with a widened token.
- **Actions pinned by commit SHA.** Every `uses:` line names the full 40-character commit SHA with the release tag in a trailing comment, for example `actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1`. Resolve the SHA with `gh api repos/<owner>/<action>/commits/<tag> --jq .sha`, which returns the commit even for an annotated tag. Dependabot (`github-actions` ecosystem) updates the SHA and the comment together. Every `github/codeql-action` step of a workflow (`init`, `autobuild` and `analyze` in `codeql.yml`) runs the same version, SHA and comment alike, because `analyze` rejects the configuration an `init` of another version wrote; `upload-sarif` in `govulncheck.yml` follows the same release. Dependabot bumps all of them in one grouped PR (group `codeql-action`, pattern `github/codeql-action*`). Dependabot reads `.github/dependabot.yml` from `main`, so a change to it takes effect only after the next release merges `develop` into `main`; until then, move every step by hand in one PR. `test/workflows` fails `go test ./...` when two `github/codeql-action` steps name different refs or version comments, in one file or across files (every workflow, composite action and local action or reusable workflow they reach, `jobs.*.steps` and `runs.steps` alike), and when the `github-actions` entry of `.github/dependabot.yml` has no group matching the four sub-actions. It also fails when a remote `uses:` (anything but `./<path>` and `docker://<image>`) in `.github/workflows/*.yml`, `.github/actions/*/action.yml` or a local action or reusable workflow they reach does not end in `@` and 40 lower-case hex characters followed by a `# v<version>` comment, so a tag (`@v4`), a branch, a short SHA, a missing `@<ref>` or a missing comment is caught.
- **No expressions in script source.** A `${{ }}` inside a `run:` block, or inside the `script:` input of `actions/github-script`, is pasted into the script before the shell or Node parses it, so a secret, a step output or a value such as `github.ref_name` becomes source code. Pass the value through the step's `env:` and reference it as a quoted shell variable (`"$VERSION"`) or as `process.env.VERSION`. The same holds for the steps of a composite action (`runs.steps`) and for the `runs.args` entries and `runs.entrypoint` of a docker action. Expressions in `env:`, `if:` and other `with:` inputs are fine. A value read from outside the repository is checked before it reaches `GITHUB_OUTPUT`: bump-go runs `script/check-go-version.sh` on the versions it reads from go.dev and `go.mod`, both before the first write to `GITHUB_OUTPUT`; aur and copr run `script/check-release-version.sh` on the version taken from the release tag, and aur reads each checksum from `checksums.txt` with `script/select-checksum.sh`, which takes only the one line naming the exact archive (`copia-cli_<version>_linux_<arch>.tar.gz`), then runs `script/check-sha256.sh` on it. A guard prints the value it rejects through `workflow_escape` from `script/lib/workflow-escape.sh` (`%` as `%25`, CR as `%0D`, LF as `%0A`), so the value cannot add a workflow command to the runner log; a new `script/check-*.sh` sources it the same way. `test/workflows` fails `go test ./...` on any `${{` inside a `run:` block or a github-script `script:` input, or a docker action's `runs.args` or `runs.entrypoint`, in `.github/workflows/*.yml`, `.github/actions/*/action.yml` and every local action or reusable workflow they reach through `uses: ./<path>`, at any depth. A `uses: ./<path>` without `<path>/action.yml` or `<path>/action.yaml` (or, for a reusable workflow, without the file) fails the test too.
- **Go cache only in unprivileged jobs.** `actions/setup-go` restores and saves the Go module and build cache by default, and a tag or scheduled run restores the cache the default branch saved. The `test` job of `ci.yml` writes that cache, so a job that holds a secret or a write token must not build from it: every `actions/setup-go` step in a job that mentions `secrets.` in its `env` or in a step's `env`, `with` or `run`, or whose effective `permissions` grant `write` on any scope other than `id-token` (or `write-all`), sets `with: cache: false`. A job that such a job lists in `needs:`, directly or through other jobs, counts as privileged too, because the privileged job uses its output: `deploy` in `docs.yml` publishes what `build` generates. Today that is `release` in `release.yml`, `integration` in `ci.yml` and `integration-scheduled.yml`, `bump-go` in `bump-go.yml`, `govulncheck` in `govulncheck.yml` and `build` in `docs.yml`. The `test` job of `ci.yml` keeps its cache: it holds only `id-token: write` for the Codecov OIDC upload, and no privileged job needs it (`integration` runs in parallel, it takes nothing from `test`). `test/workflows` fails `go test ./...` on any such step without `cache: false`.
- **Pinned tools in the cached job.** The `test` job of `ci.yml` runs only pinned tools, so the cache it saves never holds whatever a tool's latest release ran: `golangci/golangci-lint-action` uses `version:` equal to `GOLANGCI_LINT_VERSION` in `.devcontainer/Dockerfile`, and `govulncheck` runs as `go run golang.org/x/vuln/cmd/govulncheck@<version>`, the same version in `ci.yml` and `govulncheck.yml`, and both `codecov/codecov-action` steps set the Codecov CLI `version:` (`v11.3.2`) instead of the action's default `latest`. The `release` job of `release.yml` holds `contents: write` and `HOMEBREW_TAP_TOKEN`, so `goreleaser/goreleaser-action` runs an exact goreleaser `version:` (`v2.18.3`), not `latest` and not the action's floating default `~> v2`. Dependabot does not track `with.version` or a `go run ...@version`, so bump them by hand: change `GOLANGCI_LINT_VERSION` and the action's `version:` together, set the goreleaser `version:` only to a release that is neither a prerelease nor mutable (read the target tag first with `gh release view <tag> -R goreleaser/goreleaser --json isPrerelease,isImmutable`, or the `prerelease` and `immutable` fields of `gh api repos/goreleaser/goreleaser/releases/tags/<tag>`, and bump only on `"isPrerelease":false` and `"isImmutable":true`), and set the Codecov `version:` of both steps to the same Codecov CLI release (`https://cli.codecov.io/api/linux/latest` names the current one). `test/workflows` fails `go test ./...` when the two golangci-lint versions differ, when a `run:` block of a workflow uses `@latest`, when a `codecov/codecov-action` or `goreleaser/goreleaser-action` step, or any other step that sets `with.version`, has no `version:` matching `^v\d+\.\d+\.\d+$` (so `latest`, `v2`, `~> v2`, `nightly` and an empty value fail), when any step sets `with.version-file`, and when a `codecov/codecov-action` step sets `binary`, or `skip_validation` or `use_pypi` to any value but `false` (each skips the GPG signature and SHA256 check of the Codecov CLI, and `binary` replaces the pinned CLI); input names match regardless of case, as on the runner, and it checks every workflow, composite action and local action it reaches.
- **One Go version.** The `go` directive of `go.mod`, the `FROM golang:<version>-alpine` line of the root `Dockerfile` and that of `.devcontainer/Dockerfile` name the same Go version. The sandbox image sets `GOTOOLCHAIN=local`, so an image on an older Go cannot build the module, and golangci-lint, built with the image's Go, refuses a newer target; CI does not notice, because `actions/setup-go` reads `go.mod`. A Go bump changes all three in one PR. `test/workflows` fails `go test ./...` when either Dockerfile names another version than `go.mod`, or has no `FROM golang:<version>-alpine` line.

The release jobs in `aur.yml` and `copr.yml` hold publishing secrets, so they run the version gate (the step with `id: version`) right after `actions/checkout`, before they install anything, and check out with `persist-credentials: false`, because neither runs authenticated git against GitHub. copr installs copr-cli only from `rpmbuild/requirements.txt` with `pip3 install --require-hashes --no-build-isolation -r rpmbuild/requirements.txt`: the file pins copr-cli and every package of its dependency tree to one version with the sha256 hash of every distribution, and `--no-build-isolation` builds the sdist-only packages with the runner's apt `python3-setuptools` and `python3-wheel` instead of fetching unpinned build tools. Dependabot (`pip` ecosystem, directory `/rpmbuild`) updates it. To refresh it by hand, edit `rpmbuild/requirements.in` if needed and run, from `rpmbuild/`:

```bash
uv pip compile --generate-hashes --python-version 3.12 --python-platform x86_64-unknown-linux-gnu requirements.in -o requirements.txt
```

(`pip-compile --generate-hashes requirements.in` from pip-tools, run under Python 3.12, works too.) `test/workflows` fails `go test ./...` when copr.yml names a package on a `pip install` line, installs without `--require-hashes -r <file>`, or the file has a requirement without `==` and `--hash=sha256:`; when the version step does not follow the checkout; when a release checkout keeps its credentials; and when `.github/dependabot.yml` has no `pip` entry for the file's directory.

copr.yml runs only on a release, so the `copr-install` job of `ci.yml` runs the same `Install copr-cli` step, then `copr-cli --help`, on every pull request and push to `main` and `develop`. A Dependabot or manual refresh of `rpmbuild/requirements.txt` that drops, adds or mis-hashes a package therefore fails on its own PR instead of in the next release. The job holds no secrets and checks out with `persist-credentials: false`. `test/workflows` fails `go test ./...` when the `Install copr-cli` run blocks of `ci.yml` and `copr.yml` differ, so change both together, and when a step of `copr-install` mentions `secrets.` in `run`, `with` or `env`.

## Reporting Bugs

Please open an issue with:

- The command you ran
- What happened (include error output)
- What you expected to happen
- Your OS and `copia-cli --version` output
