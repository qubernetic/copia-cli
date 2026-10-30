# gh CLI Feature Parity Tracker

Mapping of GitHub CLI (`gh`) commands to Copia CLI equivalents. Tracks what is achievable via Gitea API and implementation status.

## Legend

- **Full** — Gitea API provides complete equivalent
- **Partial** — Possible but with limitations
- **None** — No Gitea API equivalent
- [ ] — Not implemented
- [x] — Implemented

## auth

| gh command | Parity | Copia CLI | Status |
|-----------|--------|-----------|--------|
| `gh auth login` | Full | `copia-cli auth login` | [x] |
| `gh auth logout` | Full | `copia-cli auth logout` | [x] |
| `gh auth status` | Full | `copia-cli auth status` | [x] |
| `gh auth token` | Full | `copia-cli auth token` | [ ] |
| `gh auth setup-git` | None | n/a | — |

## repo

| gh command | Parity | Copia CLI | Status |
|-----------|--------|-----------|--------|
| `gh repo list` | Full | `copia-cli repo list` | [x] |
| `gh repo view` | Full | `copia-cli repo view` | [x] |
| `gh repo create` | Full | `copia-cli repo create` | [x] |
| `gh repo clone` | Full | `copia-cli repo clone` | [x] |
| `gh repo fork` | Full | `copia-cli repo fork` | [x] |
| `gh repo delete` | Full | `copia-cli repo delete` | [x] |
| `gh repo archive` | Partial | `copia-cli repo archive` | [ ] |
| `gh repo rename` | Full | `copia-cli repo rename` | [ ] |
| `gh repo edit` | Full | `copia-cli repo edit` | [ ] |
| `gh repo set-default` | n/a | `copia-cli repo set-default` | [ ] |

## issue

| gh command | Parity | Copia CLI | Status |
|-----------|--------|-----------|--------|
| `gh issue list` | Full | `copia-cli issue list` | [x] |
| `gh issue create` | Full | `copia-cli issue create` | [x] |
| `gh issue view` | Full | `copia-cli issue view` | [x] |
| `gh issue close` | Partial | `copia-cli issue close` | [x] |
| `gh issue reopen` | Full | `copia-cli issue reopen` | [ ] |
| `gh issue comment` | Full | `copia-cli issue comment` | [x] |
| `gh issue edit` | Full | `copia-cli issue edit` | [x] |
| `gh issue delete` | Full | `copia-cli issue delete` | [ ] |
| `gh issue pin` | Full | `copia-cli issue pin` | [ ] |
| `gh issue lock` | None | n/a | — |
| `gh issue transfer` | None | n/a | — |

`gh issue close`: Gitea records no close reason (`completed` / `not planned`), so there is no `--reason`. Write the reason with `--comment`.

## pr

| gh command | Parity | Copia CLI | Status |
|-----------|--------|-----------|--------|
| `gh pr list` | Full | `copia-cli pr list` | [x] |
| `gh pr create` | Full | `copia-cli pr create` | [x] |
| `gh pr view` | Full | `copia-cli pr view` | [x] |
| `gh pr merge` | Full | `copia-cli pr merge` | [x] |
| `gh pr close` | Full | `copia-cli pr close` | [x] |
| `gh pr reopen` | Full | `copia-cli pr reopen` | [ ] |
| `gh pr diff` | Full | `copia-cli pr diff` | [x] |
| `gh pr checkout` | Partial | `copia-cli pr checkout` | [x] |
| `gh pr review` | Full | `copia-cli pr review` | [x] |
| `gh pr checks` | Partial | `copia-cli pr checks` | [ ] |
| `gh pr comment` | Full | `copia-cli pr comment` | [ ] |
| `gh pr edit` | Full | `copia-cli pr edit` | [ ] |
| `gh pr ready` | Full | `copia-cli pr ready` | [ ] |

## release

| gh command | Parity | Copia CLI | Status |
|-----------|--------|-----------|--------|
| `gh release list` | Full | `copia-cli release list` | [x] |
| `gh release create` | Full | `copia-cli release create` | [x] |
| `gh release view` | Full | `copia-cli release view` | [ ] |
| `gh release delete` | Full | `copia-cli release delete` | [x] |
| `gh release upload` | Full | `copia-cli release upload` | [x] |
| `gh release download` | Full | `copia-cli release download` | [ ] |
| `gh release edit` | Full | `copia-cli release edit` | [ ] |

## label

| gh command | Parity | Copia CLI | Status |
|-----------|--------|-----------|--------|
| `gh label list` | Full | `copia-cli label list` | [x] |
| `gh label create` | Full | `copia-cli label create` | [x] |
| `gh label edit` | Full | `copia-cli label edit` | [ ] |
| `gh label delete` | Full | `copia-cli label delete` | [ ] |

## Other implemented commands

| Command | Status |
|---------|--------|
| `copia-cli api` | [x] |
| `copia-cli search repos` | [x] |
| `copia-cli search issues` | [x] |
| `copia-cli org list` | [x] |
| `copia-cli org view` | [x] |
| `copia-cli notification list` | [x] |
| `copia-cli notification read` | [x] |
| `copia-cli browse` (`gh browse`: repository, issue or PR number, `--no-browser`; no file paths, `--branch`, `--commit`, `--settings`, `--wiki`) | [x] |
| `copia-cli completion` | [x] |

## Other gh commands (not yet implemented)

| gh command group | Parity | Notes |
|-----------------|--------|-------|
| `gh ssh-key` | Full | `GET/POST /user/keys` |
| `gh gpg-key` | Full | `GET/POST /user/gpg_keys` |
| `gh status` | Partial | Compose from multiple endpoints |
| `gh gist` | None | No Gitea equivalent |
| `gh workflow` / `gh run` | None | Gitea Actions exist but Copia may not expose them |
| `gh codespace` | None | GitHub-specific |
| `gh copilot` | None | GitHub-specific |
| `gh project` | None | GitHub Projects, no Gitea equivalent |
| `gh cache` | None | GitHub Actions cache |
| `gh attestation` | None | GitHub-specific |
| `gh ruleset` | None | Gitea has branch protection via different API |
| `gh extension` | None | gh plugin system |

## Summary

| Status | Count |
|--------|-------|
| Implemented | 36 |
| Not yet implemented (Full parity) | 12 |
| Not yet implemented (Partial) | 4 |
| No equivalent | 14 |
| **Total implementable** | **52** |
