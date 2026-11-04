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
| `gh repo sync` | Partial | `copia-cli repo sync` | [ ] |
| `gh repo deploy-key` | Full | `copia-cli repo deploy-key` | [ ] |

`gh repo sync`: `POST /repos/{owner}/{repo}/merge-upstream` (`branch`, `ff_only`) brings a branch of a fork up to date with its upstream on the server. There is no `--force` (hard reset) and no sync of a local clone, which `gh` does with git.

`gh repo deploy-key`: `GET` and `POST /repos/{owner}/{repo}/keys`, `DELETE /repos/{owner}/{repo}/keys/{id}` for list, add and delete.

## issue

| gh command | Parity | Copia CLI | Status |
|-----------|--------|-----------|--------|
| `gh issue list` | Full | `copia-cli issue list` | [x] |
| `gh issue create` | Full | `copia-cli issue create` | [x] |
| `gh issue view` | Full | `copia-cli issue view` | [x] |
| `gh issue close` | Partial | `copia-cli issue close` | [x] |
| `gh issue reopen` | Full | `copia-cli issue reopen` | [x] |
| `gh issue comment` | Full | `copia-cli issue comment` | [x] |
| `gh issue edit` | Full | `copia-cli issue edit` | [x] |
| `gh issue delete` | Partial | `copia-cli issue delete` | [x] |
| `gh issue pin` | Partial | `copia-cli issue pin` | [x] |
| `gh issue unpin` | Partial | `copia-cli issue unpin` | [x] |
| `gh issue lock` | Partial | `copia-cli issue lock` | [x] |
| `gh issue unlock` | Partial | `copia-cli issue unlock` | [x] |
| `gh issue transfer` | None | n/a | — |

`gh issue close`: Gitea records no close reason (`completed` / `not planned`), so there is no `--reason`. Write the reason with `--comment`.

`gh issue delete`: `DELETE /repos/{owner}/{repo}/issues/{index}`. Gitea 1.26 lets only repository admins delete an issue; others get HTTP 403. app.copia.io refuses the request with HTTP 405 (found by the `integration` job of #368), so on Copia the command reports that and the issue stays; close it with `issue close` instead. There is no interactive prompt, so the command needs `--yes`.

`gh issue pin`, `gh issue unpin`: `POST` and `DELETE /repos/{owner}/{repo}/issues/{index}/pin`. Both read the issue first and leave it alone when its `pin_order` already shows the wanted state, as `gh` does; a server that sends no `pin_order` gets the request anyway. Gitea needs write access to the repository's issues for both. app.copia.io answers both `POST` and `DELETE` on the route with HTTP 404 for an issue it serves, and its issues carry no `pin_order` (found by the `integration` job of #369), so on Copia both commands report that the host does not support pinning and the issue stays as it is.

`gh issue lock`, `gh issue unlock`: `PUT /repos/{owner}/{repo}/issues/{index}/lock` with `lock_reason`, `DELETE` on the same path. `--reason/-r` takes gh's values `off_topic`, `resolved`, `spam` and `too_heated`, sent as Gitea's default lock reasons `Off-topic`, `Resolved`, `Spam` and `Too heated`; Gitea's API does not check the reason. Both read the issue's `is_locked` first. Gitea ignores a lock request for a locked issue, so `lock` fails on one, as `gh` does without a prompt; `unlock` leaves an unlocked issue alone. Gitea needs write access to the repository's issues for both. app.copia.io answers both `PUT` and `DELETE` on the route with HTTP 404 for an issue it serves (found by the `integration` job of #370), so on Copia `lock` reports that the host does not support locking and the issue stays unlocked; `unlock` finds nothing locked and sends nothing.

`gh issue transfer`: Gitea 1.26 has no endpoint that moves an issue to another repository.

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
| `gh pr update-branch` | Full | `copia-cli pr update-branch` | [ ] |

`gh pr update-branch`: `POST /repos/{owner}/{repo}/pulls/{index}/update` with `style=merge`, or `style=rebase` for `--rebase`.

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
| `gh workflow` | Full | `/repos/{owner}/{repo}/actions/workflows`: list, view, `run` (`/{workflow_id}/dispatches`), `enable`, `disable`. Whether Copia enables Actions is unknown |
| `gh run` | Partial | `/repos/{owner}/{repo}/actions/runs`: list, view, delete, `rerun` (`/rerun`, `/rerun-failed-jobs`, `/jobs/{job_id}/rerun`), job logs (`/actions/jobs/{job_id}/logs`), `download` (`/actions/artifacts`), `watch` by polling. No `cancel`: Gitea 1.26 has no endpoint for it. Whether Copia enables Actions is unknown |
| `gh secret` | Partial | `/repos/{owner}/{repo}/actions/secrets`, `/orgs/{org}/actions/secrets`, `/user/actions/secrets/{secretname}`: list, set, delete; user secrets cannot be listed, no environment secrets. Whether Copia enables Actions is unknown |
| `gh variable` | Partial | `/repos/{owner}/{repo}/actions/variables`, `/orgs/{org}/actions/variables`, `/user/actions/variables`: list, get, set, delete; no environment variables. Whether Copia enables Actions is unknown |
| `gh search code` | None | No code search endpoint in Gitea 1.26 |
| `gh search commits` | None | No commit search endpoint in Gitea 1.26 |
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
| Implemented | 43 |
| Not yet implemented (Full parity) | 18 |
| Not yet implemented (Partial) | 7 |
| Not yet implemented (client side, `repo set-default`) | 1 |
| No equivalent | 12 |
| **Total implementable** | **69** |

Counted from the rows above: every `[x]` row, and every other row by its Parity value. The Parity values follow the Gitea 1.26 API (`release/v1.26` swagger); `TestAPI_Version` in `test/integration/api_test.go` logs what Copia's `GET /version` reports in the PR `integration` job. On 2026-11-04 that was `0.59.0`, Copia's own version number, not a Gitea release, so the API does not show which Gitea version Copia is built on; a command's integration test shows whether Copia serves its endpoint.
