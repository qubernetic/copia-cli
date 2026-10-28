# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `--limit/-L` (default 30) on `label list`, `org list` and `notification list`; all three page through results, so they no longer stop at the server's first page. `notification list --all` applies to every page (#245)
- `--visibility {public|private}` on `repo list`. The Gitea list endpoints take no visibility filter, so the filter runs on each page and `--limit` counts the kept repositories; paging continues past a page without a match (#172)
- `auth status` prints a `Token:` line for each host, masked with one asterisk per character of the token; `--show-token` prints the full token instead. The config file is not touched (#176)

### Security

- Go 1.26.2 → 1.26.9, fixes GO-2026-6599, GO-2026-6600, GO-2026-6603, GO-2026-6605, GO-2026-6607, GO-2026-6608, GO-2026-6610, GO-2026-6611, GO-2026-6612, GO-2026-6613 and GO-2026-6617 in the standard library (#228)
- golang.org/x/crypto 0.49.0 → 0.52.0, fixes GO-2026-5018; also pulls golang.org/x/sys 0.42.0 → 0.45.0 (#228)
- aur and copr fail when the version taken from the release tag does not match `^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$` (`script/check-release-version.sh`), and aur fails when a checksum from `checksums.txt` does not match `^[0-9a-f]{64}$` (`script/check-sha256.sh`), so a malformed tag or checksum never reaches `PKGBUILD`, `.SRCINFO` or the RPM spec. aur writes its SSH key under `umask 077` (#273)
- aur pushes to AUR only when the host presents the Ed25519 key pinned in `.github/aur_known_hosts` (`StrictHostKeyChecking=yes`), instead of trusting whatever key `ssh-keyscan` returned at run time; no workflow may use `ssh-keyscan` (#281)
- copr installs copr-cli from `rpmbuild/requirements.txt`, which pins copr-cli and every package of its dependency tree to one version with the sha256 hash of each distribution (`pip3 install --require-hashes --no-build-isolation`), instead of whatever `pip3 install copr-cli` resolved at run time in the job that holds `COPR_TOKEN`. Dependabot (`pip`, `/rpmbuild`) keeps the file current. copr runs the version gate right after the checkout, before it installs anything, and the aur and copr checkouts set `persist-credentials: false`, so the `GITHUB_TOKEN` is not left in the git credential store (#293)
- CI runs the hash-pinned copr-cli install from copr.yml on every pull request (job `copr-install`, no secrets, `persist-credentials: false`), so a refresh of `rpmbuild/requirements.txt` that breaks `pip3 install --require-hashes` fails on its own PR instead of in the next release. `test/workflows` keeps the `Install copr-cli` step of `ci.yml` and `copr.yml` identical (#295)
- The `release`, `integration`, scheduled `integration`, `bump-go` and `govulncheck` jobs set `cache: false` on `actions/setup-go`, so a job that holds a secret or a write token no longer restores the Go module and build cache the `test` job of `ci.yml` saves after running `golangci-lint` and `govulncheck@latest`. `test/workflows` fails on any `setup-go` step with the cache on in a job that mentions `secrets.` or has a `write` permission other than `id-token` (#297)
- The `build` job of `docs.yml` sets `cache: false` on `actions/setup-go`, so the docs that `deploy` publishes on GitHub Pages are no longer generated from the Go cache the `test` job of `ci.yml` saves. `test/workflows` treats a job that a privileged job lists in `needs:`, at any depth, as privileged. The `integration` job of `ci.yml` no longer waits for `test` (`needs: test` removed): it takes nothing from `test`, it does its own checkout with `cache: false`, and the edge would otherwise make `test` privileged and cost it its cache. The `test` job runs only pinned tools: golangci-lint `v2.15.0`, kept equal to `GOLANGCI_LINT_VERSION` in `.devcontainer/Dockerfile`, and `govulncheck@v1.9.0` in `ci.yml` and `govulncheck.yml`; `test/workflows` fails on `@latest` in any workflow `run:` block (#299)
- The `release` job of `release.yml` runs goreleaser `v2.18.3` instead of `version: latest`, so a tag release no longer downloads whatever goreleaser was published last and runs it with `contents: write` and `HOMEBREW_TAP_TOKEN`. `test/workflows` fails on `with.version: latest` in any workflow step (#301)
- The `init`, `autobuild` and `analyze` steps of `codeql.yml` run `github/codeql-action` 4.39.1 together, and Dependabot bumps every `github/codeql-action*` step in one grouped PR (`codeql-action`), so a bump no longer splits into one PR per step that each fails CodeQL on mismatched versions. `test/workflows` fails when two `github/codeql-action` steps of one workflow differ in ref or version comment, and when the `github-actions` Dependabot entry has no group for them (#312)

### Fixed

- `test/workflows` checks every local action a workflow uses, not only `.github/actions/*/action.yml`: it follows each `uses: ./<path>` (also from one local action to the next, and to reusable workflows), fails when the path has no `action.yml` or `action.yaml`, and flags `${{` in a docker action's `runs.args` and `runs.entrypoint` (#279)
- aur reads each Linux checksum from `checksums.txt` with `script/select-checksum.sh`, which takes only the one line whose name equals `copia-cli_<version>_linux_<arch>.tar.gz` and fails on a missing or duplicated line. A line for another version, a longer name such as `...tar.gz.sbom.json`, or a line containing both archive names can no longer put a wrong checksum into `PKGBUILD` (#283)
- `script/check-release-version.sh` accepts only `X.Y.Z`, so aur and copr reject a pre-release version such as `1.2.3-rc.1`, which makepkg and rpmbuild cannot package (#282)
- `script/check-go-version.sh` prints a rejected value with the workflow-command encoding (`%` as `%25`, CR as `%0D`, LF as `%0A`), so a value with a line break can no longer put a second workflow command into the runner log. It also rejects version components with a leading zero or more than four digits, such as `01.27` (#271)
- `script/check-sha256.sh` and `script/check-release-version.sh` print a rejected value with the same workflow-command encoding, so duplicate lines in `checksums.txt` can no longer put a second workflow command into the runner log. All three `script/check-*.sh` guards share the encoding in `script/lib/workflow-escape.sh` (#278)
- `close-issues-on-merge.yml` moves to revision 2 of the organisation's canonical text (horde-ci baseline item 11): a closing keyword counts only at the start of a line of the PR body, optionally after a `-` or `*` list marker, so `security fix #32` inside a sentence no longer closes #32; an issue that cannot be closed fails the run instead of leaving a warning (#303)

### Changed

- `.devcontainer/compose.yaml` adds a `dev` service for the Horde sandbox (`horde-dev`): it runs as `1000:100`, keeps the Go caches and `HOME` on named volumes, is limited to 2 GB of memory and ships golangci-lint v2.15.0. bump-go now also updates `.devcontainer/Dockerfile`. The local workflow (`docker-compose*.yml`, `devcontainer.json`, `justfile`) is unchanged (#257)
- All workflows except bump-go read the Go version from `go.mod` instead of a hard-coded `1.26` (#228)
- bump-go opens its pull requests with a GitHub App token, so CI runs on them; its actions are pinned by commit SHA and its `GITHUB_TOKEN` is read-only. Setup and key rotation are in `docs/ci.md` (#238)
- CI measures coverage across packages (`go test -coverpkg=./...`), so code tested from another package counts; unit tests now run every command end to end, `repo clone` and `pr checkout` against a real local git repository, and test coverage is raised to 60% or more (#165)
- `codecov.yml` adds a coverage gate: `codecov/patch` fails when less than 80% of a PR's changed lines are covered. It is not a required check; see `CONTRIBUTING.md` (#255)
- CI uploads coverage to Codecov with GitHub OIDC instead of a token. The upload uses `codecov/codecov-action` v7.1.1 pinned by commit SHA. Tests run through gotestsum, and a second upload sends the JUnit report to Codecov Test Analytics. PRs now also get a `codecov/project` status from the organisation's Codecov configuration: it fails when total coverage drops by more than 1% (#262)
- The unit-test HTTP mock rejects requests that send a token as `?token=` or `?access_token=`; a new integration test checks that Copia refuses them with 403. The docs now state that header auth is the only supported method on Copia since 2026-05-01 (#235)
- The organisation's canonical `close-issues-on-merge.yml` replaces `close-linked-issues.yml`: a PR merged into `develop` closes the issues named in its body's `Closes #N` lines with the comment `Fixed in develop by <sha> (PR #<n>).`; merges into other branches close nothing. The action is pinned by commit SHA and the token defaults to read-only (#264)
- Every workflow sets a read-only `GITHUB_TOKEN` at the top level and widens it per job only where a step needs it; every action is pinned by commit SHA with its tag in a comment (`winget-releaser` moves from `main` to the same commit as `v2`). The scheduled integration test no longer holds `issues: write`: a separate `report-failure` job, which checks out no code, opens the failure issue. The rule is in `CONTRIBUTING.md` (#266)
- bump-go, aur and copr pass secrets, step outputs and `github.ref_name` to their `run:` steps through `env:` and quoted shell variables instead of `${{ }}` in the script. bump-go fails when the version read from go.dev does not match `^[0-9]+\.[0-9]+(\.[0-9]+)?$` (`script/check-go-version.sh`). `go test ./...` fails on a `${{` inside any `run:` block; the rule is in `CONTRIBUTING.md` (#268)
- The workflow expression check also fails on a `${{` in the `script:` input of an `actions/github-script` step, and scans the steps of composite actions under `.github/actions/*/action.yml` (#270)
- Only a repository admin can create a `v*` release tag (new `Release tag creation` ruleset); pushed `v*` tags stay undeletable and immovable for everyone. `docs/releasing.md` and `CONTRIBUTING.md` state the rule (#272)

### Fixed

- `issue create --label` and `issue edit --add-label` resolve names against the organization labels as well as the repository labels, so org-level labels can be attached; a repository label wins on a name clash. `issue create` now sends label IDs instead of raw names (#216)
- `issue edit --add-label` looks up label names on every page of the repository labels, so labels past the first 50 resolve instead of failing with "not found" (#246)
- `issue list` pages through results, so `--limit` above 50 returns up to that many issues instead of stopping at the first page; `--state` and `--label` apply to every page (#215)
- `pr list`, `repo list` (user and `--org`), `release list`, `search issues` and `search repos` page through results the same way, so `--limit` above 50 returns up to that many items; filters and the search query apply to every page (#244)
- Linux install guide now points Fedora/RHEL users to the `cbiro/copia-cli` COPR project; `qubernetic/copia-cli` does not exist (#233)

## [1.1.0] - 2026-04-16

### Added

- WinGet automated publishing on release (#18, #190)
- COPR package for Fedora/RHEL — `dnf copr enable qubernetic/copia-cli` (#164, #193)
- AUR package for Arch Linux — `yay -S copia-cli-bin` (#161, #194)
- Snap package for Ubuntu/Linux — `snap install copia-cli --classic` (#162, #195)
- Scheduled weekly integration test with auto issue on failure (#134, #192)
- Tag protection ruleset for `v*` release tags (#149)

### Changed

- Dev environment migrated to Aurora-DX: Docker Compose + justfile, Makefile removed (#184, #185)
- Bump-go workflow now also updates Dockerfile (#188, #189)
- Installation docs updated with all new package managers (#191, #198)
- Maintainer email updated to info@qubernetic.com (#196, #197)

### Fixed

- AUR workflow: pkgbuild input expects file path, not inline content (#196)
- COPR workflow: heredoc indentation broke INI parser (#196)
- WinGet workflow: use ubuntu-slim per official docs (#196)
- Bump-go workflow: add setup-go step for go mod tidy (#196)

### Dependencies

- Go 1.26.1 → 1.26.2 (#186, #187)
- github.com/spf13/pflag 1.0.9 → 1.0.10 (#179)
- actions/github-script 8 → 9 (#183)
- actions/upload-pages-artifact 4 → 5 (#182)
- codecov/codecov-action 5 → 6 (#180)

---

## [1.0.0] - 2026-04-03

### Highlights

First stable release of Copia CLI — a command-line interface for Copia, the source control platform for industrial automation. Modeled after GitHub CLI (`gh`), with 35+ subcommands across 11 command groups.

### Added

- Version command enrichment: Go version, OS/arch (#150)
- Test coverage reporting with Codecov (#151)
- GoDoc package comments on all exported packages (#152)
- Shell completion detailed setup instructions (#156)
- Branch protection rulesets on main/develop
- Enriched command Long descriptions with filtering details (#138)
- Integration tests for org, search, notification, api, release (#144)

### Since v0.5.0-rc.2

All v0.5.0-rc.2 changes are included. See below for the full pre-release changelog.

---

## [0.5.0-rc.2] - 2026-04-02

### Added

- Release variants: .deb, .rpm packages via nfpms
- macOS universal binary (amd64+arm64 fat binary)
- linux/386, linux/arm (v6), windows/386 build targets
- `-R`/`--repo` flag for all repo-scoped commands (#105)
- Jekyll manual site matching gh CLI layout (#132)
- Command group annotations (General/Targeted) on parent commands
- Long descriptions and descriptive examples for all commands
- Platform install guides (Linux, macOS, Windows, source)
- `notification list --all` flag (#109)
- `issue list --label` flag (#111)
- `pr close --comment` and `--delete-branch` flags (#112)

### Fixed

- `auth login` failed with "file already closed" due to premature response body close (#97)
- `resp.Body.Close` before `ReadAll` in all HTTP commands (#100)
- `repo clone` failed on private repos — missing auth (#101)
- PATCH/POST commands rejected valid 201 status from Gitea API (#102)
- `--state` flag not validated in issue/pr list (#106)
- `auth status` did not error for unknown host (#107)
- `search issues` returned empty results — defaulted to open only (#108)
- `notification list` HTTP 500 without page parameter (#109)
- `--limit` accepted negative values (#110)
- GoReleaser archive format deprecation warnings (#96)

### Changed

- License changed from MIT to AGPL-3.0 + Commercial dual license (#135)
- Documentation migrated from mdBook to Jekyll (#132)
- README restructured to match gh CLI lean style (#83)
- Integration tests rewritten to exercise CLI code paths (#103)
- `docs/` reorganized to match gh CLI convention

## [0.4.0-rc.1] - 2026-04-02

### Changed

- Binary renamed from `copia` to `copia-cli` (avoids conflict with Copia Desktop)
- Organization renamed from `qubernetic-org` to `qubernetic`
- Go module path: `github.com/qubernetic/copia-cli`

### Fixed

- Auth precedence: flag > env var > config (env vars were silently ignored)
- BaseRepo detection from git remote origin (repo-scoped commands now work)
- Errors printed to stderr (were silently swallowed)
- Clone git flag injection (added `--` separator)
- Interactive login token input (bufio.Scanner + trim)
- Error messages no longer reference non-existent `--repo` flag
- Search issues uses correct per-repo endpoint
- Issue edit `--add-label` resolves label IDs by name
- JSON field names aligned to snake_case
- `splitOwnerRepo` deduplicated to `cmdutil.SplitOwnerRepo`
- `ApiOptions` renamed to `APIOptions` (Go convention)

## [0.3.0-beta.1] - 2026-04-01

### Added

- `copia api` — generic REST escape hatch with --field, --header, --method
- `copia search repos` — search repositories across the instance
- `copia search issues` — search issues with --state filter
- `copia org list` — list user's organizations
- `copia org view` — view organization details
- `copia notification list` — list unread notifications
- `copia notification read` — mark notifications as read (single or --all)
- `copia completion` — shell completion for bash, zsh, fish, powershell
- User manual website (mdBook) with auto-generated command reference
- GitHub Pages deployment workflow for manual
- `make docs` target for command reference generation

## [0.2.0-beta.1] - 2026-04-01

### Added

- `copia release list` — list releases with --json
- `copia release create` — create release with --draft, --prerelease
- `copia release delete` — delete release by tag
- `copia release upload` — upload assets to release
- `copia repo create` — create repo with --org, --private
- `copia repo delete` — delete repo with --yes confirmation
- `copia repo fork` — fork repo with optional --org
- `copia pr review` — submit review (--approve, --request-changes, --comment)
- `copia pr diff` — view PR diff output
- `copia pr checkout` — check out PR branch locally
- `copia issue edit` — edit title, body, labels, assignees, milestone
- Homebrew tap distribution (`brew install qubernetic/tap/copia`)
- Go vulnerability check (govulncheck) in CI pipeline and weekly SARIF scan
- Go mod tidy check in CI
- Go version auto-bump workflow
- Dependabot target-branch set to develop

## [0.1.0-beta.1] - 2026-04-01

### Added

- `copia auth login` — authenticate with token validation
- `copia auth logout` — remove host from config
- `copia auth status` — display hosts with token validity check
- `copia repo list` — list user/org repos with --json
- `copia repo view` — view repo details with --json
- `copia repo clone` — clone via owner/repo or URL
- `copia repo create` — create repo with --org and --private
- `copia repo delete` — delete repo with --yes confirmation
- `copia repo fork` — fork repo with optional --org
- `copia issue list` — list issues with --state, --limit, --json
- `copia issue create` — create issue with --title, --body, --label
- `copia issue view` — view issue details with --json
- `copia issue close` — close issue with optional --comment
- `copia issue comment` — add comment to issue
- `copia pr list` — list PRs with --state, --limit, --json
- `copia pr create` — create PR with --title, --body, --base, --head
- `copia pr view` — view PR details with --json
- `copia pr merge` — merge with --merge/--squash/--rebase, --delete-branch
- `copia pr close` — close a PR
- `copia label list` — list repo labels with --json
- `copia label create` — create label with --name, --color, --description
- `copia release list` — list releases with --json
- `copia release create` — create release with --draft, --prerelease
- `copia release delete` — delete release by tag
- `copia release upload` — upload assets to release
- YAML config management with multi-host support (~/.config/copia/config.yml)
- TTY-aware IOStreams abstraction for testable output
- HTTP mock registry for unit testing
- Factory dependency injection (gh CLI pattern)
- --json flag on all list/view commands
- Devcontainer with Go 1.26, gh CLI, golangci-lint, Claude Code
- GoReleaser for cross-platform releases
- GitHub Actions CI (test, lint, govulncheck, integration tests)
- CodeQL security analysis
- Dependabot for Go modules and GitHub Actions
- Go vulnerability check (govulncheck) with SARIF upload
- Go version auto-bump workflow
- Auto-close linked issues on non-default branch merges
- Integration tests against live Copia API
