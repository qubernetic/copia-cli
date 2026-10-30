# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `--limit/-L` (default 30) on `label list`, `org list` and `notification list`; all three page through results, so they no longer stop at the server's first page. `notification list --all` applies to every page (#245)
- `--visibility {public|private}` on `repo list`. The Gitea list endpoints take no visibility filter, so the filter runs on each page and `--limit` counts the kept repositories; paging continues past a page without a match (#172)
- `auth status` prints a `Token:` line for each host, masked with one asterisk per character of the token; `--show-token` prints the full token instead. The config file is not touched (#176)
- `--label/-l` on `search issues` (repeatable or comma separated, sent as `labels=a,b` like `issue list --label`); results come newest first, the issue endpoint takes no sort. `--sort {alpha|created|updated|size|stars|forks}` and `--order {asc|desc}` (default `desc`) on `search repos`; `--order` is ignored without `--sort`, as in `gh search repos`, and an unknown value of either fails before any request (#171)
- `--body/-b` on `pr merge` sets the body of the merge commit (sent as `MergeMessageField`); Gitea keeps its default title and appends the body after a blank line. Combines with `--squash`, `--rebase` and `--delete-branch`; without `--body` the field is left out, so a squash merge keeps its default body (#175)
- `--comments/-c` on `issue view` and `pr view` prints every conversation comment after the body, oldest first, each as `<login> commented <created_at>:` and its text; a second request reads `/issues/{n}/comments`, which also serves pull requests. PR reviews and inline review comments are not shown. `--comments` with `--json` fails before any request; the `issue view` JSON `comments` field stays the count (#173)
- `--assignee/-a` (repeatable) and `--milestone/-m` on `issue create`. `--assignee @me` assigns the authenticated user, read with one `GET /user`. `--milestone` takes a name or a numeric ID and resolves it with `GET /repos/{owner}/{repo}/milestones/{name}`; an unknown milestone fails before the issue is created. `issue edit --milestone` uses the same lookup, so it now takes a name too, and a numeric ID works as before (#169)
- `--add-assignee` and `--remove-assignee` (repeatable) on `issue edit`. Gitea's issue PATCH replaces the whole list, so the current assignees are read with `GET /repos/{owner}/{repo}/issues/{index}` and the changed list is sent; removing a login the issue does not have changes nothing, and removing the last one sends `"assignees": []`. `--assignee` still replaces the list and fails before any request when combined with either new flag. `--remove-label` (repeatable) on `issue edit` resolves names like `--add-label` (repository and organization labels) and sends one `DELETE /repos/{owner}/{repo}/issues/{index}/labels/{id}` per label; an unknown name fails before the issue is changed (#170)
- `--assignee/-a`, `--author/-A`, `--mention`, `--milestone/-m` and `--search/-S` on `issue list`, sent as the server-side filters `assigned_by`, `created_by`, `mentioned_by`, `milestones` (the name as given) and `q` on every page. `@me` in `--assignee` and `--author` stands for the authenticated user, read with one `GET /user` even when both flags use it; without the new flags the request is unchanged (#168)
- `--assignee/-a`, `--label/-l` and `--reviewer/-r` (all repeatable) and `--milestone/-m` on `pr create`. Assignees (`@me` read with one `GET /user`), label IDs (repository and organization labels, by name) and the milestone ID (name or numeric ID) go into the create body; an unknown label or milestone fails before the pull request is created. Reviewers are requested after the create with one `POST /repos/{owner}/{repo}/pulls/{index}/requested_reviewers`: a value with a slash (`org/team`) is sent as a team in `team_reviewers`, the rest in `reviewers`. If that request fails, the pull request still exists: its URL is printed and the command exits non-zero (#166)
- `--draft/-d`, `--body-file/-F` and `--fill/-f` on `pr create`. Gitea has no draft field, so `--draft` prefixes the title with `WIP: ` unless it already starts with `WIP:` or `[WIP]` (case-insensitive). `--body-file` reads the body from a file, `-` from standard input, and fails with `--body` before any request. `--fill` takes the title and body from `git log --cherry <base>...<head>` (`origin/<base>` when it exists, the head defaulting to the current branch): one commit gives its subject and body, several commits give the branch name and a bulleted list of their subjects, oldest first. An explicit `--title` or `--body` wins, and `--title` is not required with `--fill` (#320)
- `--json` prints only the requested fields: an array of objects for a list, one object for a view. It takes a comma-separated list and can be repeated; an unknown field fails before any request and lists the available fields. Applies to `issue list`, `issue view`, `pr list`, `pr view`, `repo list`, `repo view`, `release list`, `label list`, `org list`, `org view`, `notification list`, `search repos` and `search issues` (#177)
- `--jq/-q <expression>` on the same 13 commands filters the `--json` output with `github.com/itchyny/gojq`, the jq library `gh` uses (new dependency). Each result is printed on its own line: a string raw, `null` as an empty line, anything else as compact JSON, as in `gh`. `--jq` without `--json` and an expression that does not parse fail before any request (#177)
- `--template/-t <template>` on the same 13 commands formats the `--json` output with a Go template, as in `gh`, for example `--json number --template '{{range .}}{{.number}}{{"\n"}}{{end}}'`. The helpers are `join`, `pluck`, `truncate`, `timeago`, `tablerow` and `tablerender`; the `gh` color helpers are left out. `--template` without `--json`, together with `--jq`, or a template that does not parse fails before any request (#321)
- `--raw-field/-f key=value` on `api` always sends the value as a string; `-f key=value` sends exactly what it sent before (#167)
- `--input <file>` on `api` sends the file as the request body, `-` reads standard input; the default method is then POST and the `Content-Type` stays `application/json` unless `--header` sets another. With `--input`, `--field` and `--raw-field` values are added to the query string, as in `gh` (`null` as an empty value) (#167)
- `--include/-i` on `api` prints the status line (such as `HTTP/1.1 200 OK`), the response headers sorted by name and a blank line before the body; a `204 No Content` response prints only these lines (#167)
- `--paginate` on `api` follows the `rel="next"` entry of the `Link` header until there is none and prints the items of array pages as one array, as `gh api --paginate` does without `--slurp`; any other response is printed as is. A next link to another host is refused, so the token is never sent there. `--paginate` with a method other than GET, also the POST that fields default to, fails before any request (#322)
- `--jq/-q <expression>` on `api` filters the response like `--jq` on the `--json` commands; with `--paginate` it runs on each page. An expression that does not compile fails before any request, and an error response is printed unfiltered (#322)

### Security

- Every command follows an HTTP redirect only when it keeps the scheme, host and port of the request, so the `Authorization` header never reaches another server or travels over plain HTTP. A redirect to another origin, including `https://host` to `http://host` or to `https://host:<other port>`, fails the command with an error that names both origins instead of being followed. The 10-redirect limit stays. All commands build their client with `cmdutil.NewHTTPClient()`, the uncalled `api.NewClient` is removed, and a test fails on `&http.Client{}` or `http.DefaultClient` in non-test code under `pkg/` and `internal/` (#333)
- `repo clone` sends the token only to the configured host: an `https` URL whose host equals it (case-insensitive) on the same port, 443 when none is given. Any other URL, such as another host, a look-alike host (`app.copia.io.evil.example`), `http://`, another port or `git@`, is cloned without credentials; before, every `https://` URL got the token. The token is no longer in the clone URL or git's arguments, so other local users cannot read it from the process list: git gets it from a credential helper set through `GIT_CONFIG_*` environment entries under `credential.https://<host>.helper`, which reads it from `COPIA_CLONE_TOKEN`, replaces the user's helpers for that host only, and ignores `store` and `erase`, so no credential store keeps a copy. git asks for credentials per origin, so a redirect from the configured host to another origin does not carry the token; tests pin this with two `git http-backend` servers. A configured host with a wildcard, user name, path, query or fragment is refused, and an invalid `GIT_CONFIG_COUNT` is an error instead of being read as 0; `GIT_CONFIG_*` entries already set are kept. The remote is stored as given, without the post-clone `git remote set-url` (#335)
- `--jq` keeps gojq's defaults, unlike `gh`: `$ENV` and `env` are empty, and `input`, `inputs`, `import` and `include` fail, so an expression cannot read the environment (such as `COPIA_TOKEN`) or local files; a test pins it. The filter runs with the command's context and stops when it is canceled (#322)
- Tests pin two `--jq` guarantees that nothing checked before: `api --jq` stops when the command's context is canceled, and `import` and `include` fail with gojq's own `cannot load module` error, so adding a module loader fails the suite. No behaviour change (#341)
- The HTTP client guard test reads the syntax tree instead of matching text: it fails on any `http.Client` literal (also `http.Client{}` by value or with fields such as `Timeout`) or `new(http.Client)` outside `pkg/cmdutil/httpclient.go`, and on `http.DefaultClient`, `http.Get`, `http.Head`, `http.Post` and `http.PostForm`, which send through the default client, under any local name of the `net/http` import. It now covers `cmd/` as well as `pkg/` and `internal/`. No behaviour change (#342)
- Go 1.26.2 → 1.26.9, fixes GO-2026-6599, GO-2026-6600, GO-2026-6603, GO-2026-6605, GO-2026-6607, GO-2026-6608, GO-2026-6610, GO-2026-6611, GO-2026-6612, GO-2026-6613 and GO-2026-6617 in the standard library (#228)
- golang.org/x/crypto 0.49.0 → 0.52.0, fixes GO-2026-5018; also pulls golang.org/x/sys 0.42.0 → 0.45.0 (#228)
- golang.org/x/crypto 0.52.0 → 0.56.0, fixes GO-2026-6355, GO-2026-6354 and GO-2026-6303 (listed by govulncheck as present but not called); also pulls golang.org/x/sys 0.45.0 → 0.47.0, and the `go.sum` entries of golang.org/x/term move from 0.43.0 to 0.45.0. GO-2026-5932 has no fix yet and stays (#340)
- aur and copr fail when the version taken from the release tag does not match `^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$` (`script/check-release-version.sh`), and aur fails when a checksum from `checksums.txt` does not match `^[0-9a-f]{64}$` (`script/check-sha256.sh`), so a malformed tag or checksum never reaches `PKGBUILD`, `.SRCINFO` or the RPM spec. aur writes its SSH key under `umask 077` (#273)
- aur pushes to AUR only when the host presents the Ed25519 key pinned in `.github/aur_known_hosts` (`StrictHostKeyChecking=yes`), instead of trusting whatever key `ssh-keyscan` returned at run time; no workflow may use `ssh-keyscan` (#281)
- copr installs copr-cli from `rpmbuild/requirements.txt`, which pins copr-cli and every package of its dependency tree to one version with the sha256 hash of each distribution (`pip3 install --require-hashes --no-build-isolation`), instead of whatever `pip3 install copr-cli` resolved at run time in the job that holds `COPR_TOKEN`. Dependabot (`pip`, `/rpmbuild`) keeps the file current. copr runs the version gate right after the checkout, before it installs anything, and the aur and copr checkouts set `persist-credentials: false`, so the `GITHUB_TOKEN` is not left in the git credential store (#293)
- CI runs the hash-pinned copr-cli install from copr.yml on every pull request (job `copr-install`, no secrets, `persist-credentials: false`), so a refresh of `rpmbuild/requirements.txt` that breaks `pip3 install --require-hashes` fails on its own PR instead of in the next release. `test/workflows` keeps the `Install copr-cli` step of `ci.yml` and `copr.yml` identical (#295)
- The `release`, `integration`, scheduled `integration`, `bump-go` and `govulncheck` jobs set `cache: false` on `actions/setup-go`, so a job that holds a secret or a write token no longer restores the Go module and build cache the `test` job of `ci.yml` saves after running `golangci-lint` and `govulncheck@latest`. `test/workflows` fails on any `setup-go` step with the cache on in a job that mentions `secrets.` or has a `write` permission other than `id-token` (#297)
- The `build` job of `docs.yml` sets `cache: false` on `actions/setup-go`, so the docs that `deploy` publishes on GitHub Pages are no longer generated from the Go cache the `test` job of `ci.yml` saves. `test/workflows` treats a job that a privileged job lists in `needs:`, at any depth, as privileged. The `integration` job of `ci.yml` no longer waits for `test` (`needs: test` removed): it takes nothing from `test`, it does its own checkout with `cache: false`, and the edge would otherwise make `test` privileged and cost it its cache. The `test` job runs only pinned tools: golangci-lint `v2.15.0`, kept equal to `GOLANGCI_LINT_VERSION` in `.devcontainer/Dockerfile`, and `govulncheck@v1.9.0` in `ci.yml` and `govulncheck.yml`; `test/workflows` fails on `@latest` in any workflow `run:` block (#299)
- The `release` job of `release.yml` runs goreleaser `v2.18.3` instead of `version: latest`, so a tag release no longer downloads whatever goreleaser was published last and runs it with `contents: write` and `HOMEBREW_TAP_TOKEN`. `test/workflows` fails on `with.version: latest` in any workflow step (#301)
- The `init`, `autobuild` and `analyze` steps of `codeql.yml` run `github/codeql-action` 4.39.1 together, and Dependabot bumps every `github/codeql-action*` step in one grouped PR (`codeql-action`), so a bump no longer splits into one PR per step that each fails CodeQL on mismatched versions. `test/workflows` fails when two `github/codeql-action` steps of one workflow differ in ref or version comment, and when the `github-actions` Dependabot entry has no group for them (#312)
- `repo clone` fails with `invalid token: it may contain only printable ASCII characters` before git runs when the token holds a character outside space to `~`, such as CR, LF, a tab, NUL or a non-ASCII letter. The credential helper prints the token as one `password=` line, so a line break could add credential lines such as `host=` or `url=` (#338)
- `auth status --show-token` writes one line, `warning: printing the token to a non-terminal output; it may end up in a log or a file`, to standard error when standard output is not a terminal (a pipe, a redirect or a CI log). The token is still printed, since piping it on purpose is valid; on a terminal nothing changes. `docs/authentication.md` now states that `auth status` reads the config file and `--host`, not `COPIA_HOST` or `COPIA_TOKEN` (#339)
- `test/workflows` fails when a remote `uses:` of a workflow, a composite action or a local action they reach is not pinned to a full 40-character commit SHA followed by a `# v<version>` comment, so a tag, a branch, a short SHA or a missing `@<ref>` no longer depends on review to be caught. Local actions (`./`) and `docker://` images are exempt; the workflows already comply, so no workflow changes (#343)

### Fixed

- `test/workflows` checks every local action a workflow uses, not only `.github/actions/*/action.yml`: it follows each `uses: ./<path>` (also from one local action to the next, and to reusable workflows), fails when the path has no `action.yml` or `action.yaml`, and flags `${{` in a docker action's `runs.args` and `runs.entrypoint` (#279)
- aur reads each Linux checksum from `checksums.txt` with `script/select-checksum.sh`, which takes only the one line whose name equals `copia-cli_<version>_linux_<arch>.tar.gz` and fails on a missing or duplicated line. A line for another version, a longer name such as `...tar.gz.sbom.json`, or a line containing both archive names can no longer put a wrong checksum into `PKGBUILD` (#283)
- `script/check-release-version.sh` accepts only `X.Y.Z`, so aur and copr reject a pre-release version such as `1.2.3-rc.1`, which makepkg and rpmbuild cannot package (#282)
- `script/check-go-version.sh` prints a rejected value with the workflow-command encoding (`%` as `%25`, CR as `%0D`, LF as `%0A`), so a value with a line break can no longer put a second workflow command into the runner log. It also rejects version components with a leading zero or more than four digits, such as `01.27` (#271)
- `script/check-sha256.sh` and `script/check-release-version.sh` print a rejected value with the same workflow-command encoding, so duplicate lines in `checksums.txt` can no longer put a second workflow command into the runner log. All three `script/check-*.sh` guards share the encoding in `script/lib/workflow-escape.sh` (#278)
- `close-issues-on-merge.yml` moves to revision 2 of the organisation's canonical text (horde-ci baseline item 11): a closing keyword counts only at the start of a line of the PR body, optionally after a `-` or `*` list marker, so `security fix #32` inside a sentence no longer closes #32; an issue that cannot be closed fails the run instead of leaving a warning (#303)
- `release upload` fails with `API error (HTTP <status>)` when the release lookup answers anything other than 200 or 404, as `release delete` does. A 401, 403 or 500 lookup no longer posts the file to `/releases/0/assets` and reports a misleading `uploading <file>: HTTP <status>` (#254)
- `api --paginate` fails with `the next page link repeats an already fetched page: "<url>"` when a `rel="next"` link names a page it already fetched, such as a link to the page itself or from page 2 back to page 1, instead of fetching pages forever. A table test now pins the next link origin check: plain `http://`, another port, a scheme-relative link to another host, an upper-case host and a trailing dot are refused before any request, and a `user@` link to the same host keeps the `token` header (#337)

### Changed

- `api --field` is now typed like `gh`, and its short form is `-F` (`-f` is `--raw-field`): `true`, `false`, `null` and integers are sent as JSON values, `@file` sends the contents of the file and `@-` reads standard input; anything else stays a string. A script that used `--field n=1` now sends the number `1`; use `--raw-field` or `-f` for the string. Both flags take one `key=value` per use, so a value with a comma, such as `-f title="a, b"`, is no longer split into two values (#167)
- The `--json` field names are the keys the commands print: `user` replaces `author` on `issue view`, `pr list` and `pr view`, and `stars_count` and `forks_count` replace `stars` and `forks` on `repo view`. `html_url` is now listed on `issue view`, `pr view` and `repo view`, and `id` on `release list` and `label list`; these keys were already printed. The `release list` and `repo list` help examples use `tag_name` and `full_name` (#177)
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
