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
| `gh auth switch` | Full | `copia-cli auth switch` | [x] |
| `gh auth git-credential` | Full | `copia-cli auth git-credential` | [x] |
| `gh auth token` | Full | `copia-cli auth token` | [x] |
| `gh auth setup-git` | Full | `copia-cli auth setup-git` | [x] |

The default host is the top-level config key `default_host`, which `auth login` sets to the host it logs in to; gh has no such key and defaults to `github.com` (or its only host) unless `GH_HOST` is set. The host order is `--host`, `COPIA_HOST`, `default_host`, then the alphabetically first host; `auth logout` of the default host moves `default_host` to the alphabetically first remaining host. `auth status` lists the hosts in alphabetical order and marks the default one with `(default)`.

Several accounts per host follow gh: the host entry holds `users.<login>` (a `token` only for file storage) and `user`, the active account, whose token the commands use; a config of the old shape (`token` and `user` under the host) is read as one account and saved in the new shape. `auth login` adds or updates the account and makes it active, `auth logout --user/-u <login>` removes one account (without `--user` the active one; with several accounts gh asks which one on a terminal and fails otherwise, copia-cli has no prompts), and the alphabetically first remaining account becomes active with gh's `Switched active account for <host> to <login>`. `auth status` lists every account per host, the active one first, with `Active account: true/false` as gh does. The keyring has one entry per account (service `copia-cli:<host>`, account = the login); gh also keeps an entry for the active account under an empty account name, copia-cli does not. `auth switch` follows gh's choice of the account: `--host` and `--user/-u` narrow the accounts, one remaining account is switched to, and of two the one not in use (gh: on one host only; copia-cli also toggles between two hosts, the account in use being the active account of the default host). Where gh asks on a terminal, copia-cli has no prompts and fails with the list of accounts as `<host> <login>` lines. gh only changes the active account; copia-cli also sets `default_host` to the account's host, since gh has no default host. gh refuses to switch while `GH_TOKEN` is set for the host; copia-cli switches and, when `COPIA_TOKEN` is set, warns that it applies only with `COPIA_HOST`. Like gh, which binds `GH_TOKEN` to github.com and `GH_ENTERPRISE_TOKEN` to other hosts, copia-cli binds `COPIA_TOKEN` to a host: it is used only when `--host` or `COPIA_HOST` names the host, and with the host taken from the config the stored token is used and a warning says `COPIA_TOKEN` was ignored.

Token storage follows gh: `auth login` stores the token in the OS keyring (go-keyring, service `copia-cli:<host>` where gh uses `gh:<host>`, account = the login, every call bounded by a three-second timeout as in gh's `internal/keyring`) and falls back to the config file with a warning on standard error when the keyring fails; `--insecure-storage` writes the config file on request. The stored token is read from the config file first, then from the keyring, as gh does; `auth logout` deletes it from both. `auth status` adds the source to the token line (`Token: **** (keyring)` or `(config file)`), where gh prints it after the account (`(keyring)` or `(<path to hosts.yml>)`). The snap has no `password-manager-service` plug, so it always falls back to the config file.

`auth git-credential` is hidden, as in gh, and answers git's `get` with `username=<login>` and `password=<token>` of the active account of the configured host git asks about (https only, the port must match, a missing one is 443). Like gh it reads a `url` line as well as `protocol`, `host`, `path` and `username`, refuses a request whose `username` is not the active account's login, ignores `--token` and `COPIA_TOKEN`, and does nothing for `store` and `erase`. Where gh exits 1 when it has no credential, copia-cli prints nothing and exits 0, and it prints no `protocol` and `host` lines. A token or login with a character outside printable ASCII is refused, as `repo clone` does. `auth setup-git` writes the helper into the global git config as gh does: for each logged-in host (a host with at least one account) or the one `--host` names, `git config --global --replace-all credential.https://<host>.helper ""` and then `--add` of `!'<path>' auth git-credential`. As with gh, the empty value drops only the helpers configured before it; a helper configured after it (later in the global file, repository-local, worktree or `-c`) still takes part. gh's flag is `--hostname/-h`; copia-cli uses `--host`, as its other commands do, and `--force/-f` as gh. The path is the `copia-cli` found in `PATH` when it is a symlink to the running binary (gh does the same, a hard link does not count), otherwise the running binary; gh quotes it only when it contains a space, `$` or a backslash, copia-cli always quotes it, a single quote included. gh prints nothing on success and so does copia-cli; with no host logged in gh prints a hint and exits 1, copia-cli fails with the hint as its error. gh also configures `gist.<host>`, which Copia has no equivalent of.

`auth token` prints the token for the host the commands resolve, followed by a newline, as gh does: the active account's token as the other commands would use it (`--token`, `COPIA_TOKEN` under the host rule above, then the stored token), or with `--user/-u <login>` that account's stored token. gh's `--hostname/-h` is the global `--host`. Without a token it fails with gh's message without `oauth`, `no token found for <host>` (and ` account <login>` with `--user`). copia-cli has no refreshable tokens, so gh's `--no-refresh` and its expiry warning do not apply, and no hidden `--secure-storage`, since go-gh does not read copia-cli's storage.

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
| `gh repo rename` | Full | `copia-cli repo rename` | [x] |
| `gh repo edit` | Full | `copia-cli repo edit` | [x] |
| `gh repo set-default` | n/a | `copia-cli repo set-default` | [x] |
| `gh repo sync` | Partial | `copia-cli repo sync` | [ ] |
| `gh repo deploy-key` | Full | `copia-cli repo deploy-key` | [ ] |

`gh repo view --web/-w`: client side, no API call and no token, opened as for the issue commands: `/{owner}/{repo}` of the argument, `-R` or the git remote. gh's `--branch` does not exist in `repo view`.

`gh repo set-default`: client side, no API call and no token. The default is the git config key `remote.<name>.copia-resolved` (gh uses `gh-resolved`) with the value `base`, and the commands that work on the current repository read the marked remote first, then `origin`; `-R` wins over both. The argument is `owner/repo` (any case) or a remote name, `--view/-v` prints the default and `--unset/-u` removes it. Differences from gh: there is no prompt, so without an argument the only remote is used and several remotes fail with their list; gh also offers the parent of a fork found through the API, which copia-cli does not, so the repository must be one of the git remotes; the argument fails with `--view` or `--unset`, and the messages print whether or not standard output is a terminal.

`gh repo edit [<owner/repo>]`: `PATCH /repos/{owner}/{repo}` with only the given settings: `-d/--description` as `description`, `--homepage` as `website`, `--default-branch` as `default_branch`, `--visibility public|private` as `private`, `--template` as `template`, `--enable-issues`, `--enable-wiki` and `--enable-projects` as `has_issues`, `has_wiki` and `has_projects`, `--enable-merge-commit`, `--enable-squash-merge` and `--enable-rebase-merge` as `allow_merge_commits`, `allow_squash_merge` and `allow_rebase`, and `--delete-branch-on-merge` as `default_delete_branch_after_merge` (every boolean flag takes `=false`). `--add-topic` and `--remove-topic` read `GET /repos/{owner}/{repo}/topics` and write the new list with `PUT /repos/{owner}/{repo}/topics` only when it changed; a topic is compared in lower case without surrounding space, as Copia stores it. `--visibility` needs `--accept-visibility-change-consequences`, with gh's message. app.copia.io (0.60.0 on 2026-11-07) refuses to make a repository public with 403, as Gitea does under its `FORCE_PRIVATE` setting, so `--visibility public` fails there with `failed to edit repository (HTTP 403): the server may not allow public repositories` and changes nothing; `--visibility private` works. Differences from gh: there is no prompt, so without a flag it fails with `specify properties to edit`; `--homepage` has no `-h` (copia-cli's help flag); `--visibility` takes no `internal`, since a Gitea repository is public or private; `--enable-discussions`, `--enable-auto-merge`, `--allow-forking`, `--allow-update-branch`, `--squash-merge-commit-message` and the security flags are left out (no Gitea repository setting matches them); `Edited repository <owner>/<repo>` is printed on standard output also without a terminal.

`gh repo rename <new-name>`: `PATCH /repos/{owner}/{repo}` with `{"name"}` on the current repository or `-R`, then `Renamed repository <owner>/<new-name>`. A name with `/` fails with gh's message before any API call. Without `-R` the git remote that names the old repository on the configured host (host name compared without port and case; the one marked by `repo set-default` first, then `origin`, then the first in config order) gets the new name with `git remote set-url`; only the last path segment of its configured URL changes, so scheme, user, host and a `.git` suffix stay, and `Updated the "<remote>" remote` follows. When no remote matches or git fails, `! Warning: unable to update remote ...` goes to standard error and the command still exits 0, as in gh. Differences from gh: there is no prompt, so `--yes` (`-y`) is required, also with `-R` (gh asks only without it), and the name argument is required; both success lines are printed also without a terminal; gh's deprecated `--confirm` is left out.

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
| `gh issue status` | Full | `copia-cli issue status` | [x] |
| `gh issue transfer` | None | n/a | — |

`--web/-w` on `gh issue view`, `gh issue comment`, `gh issue list` and `gh issue create`: client side, no API call and no token. The URL is built from the Gitea 1.26 web router and opened with the `browser` setting; on a terminal `Opening <url> in your browser.` goes to standard error, as in gh. `view` and `comment` open `/{owner}/{repo}/issues/{index}`. `list` opens `/{owner}/{repo}/issues` with the query parameters the web list reads: `state`, `q` (`--search`), `poster` (`--author <login>`) and `type=assigned` or `type=created_by` (`--assignee @me`, `--author @me`; the list has one `type`, so `--assignee @me` wins). The web list takes labels, milestones and other assignees only by ID and has no mention filter for another user, so `--label`, `--milestone`, `--mention` and `--assignee <login>` print a warning and are not applied. `create` opens `/{owner}/{repo}/issues/new` with `title` and `body` as query parameters, which the new-issue handler reads; it takes no labels, assignees or milestone by name, so those flags print a warning, and `--web` with `--editor` fails.

`gh issue close`: Gitea records no close reason (`completed` / `not planned`), so there is no `--reason`. Write the reason with `--comment`.

`gh issue delete`: `DELETE /repos/{owner}/{repo}/issues/{index}`. Gitea 1.26 lets only repository admins delete an issue; others get HTTP 403. app.copia.io refuses the request with HTTP 405 (found by the `integration` job of #368), so on Copia the command reports that and the issue stays; close it with `issue close` instead. There is no interactive prompt, so the command needs `--yes`.

`gh issue pin`, `gh issue unpin`: `POST` and `DELETE /repos/{owner}/{repo}/issues/{index}/pin`. Both read the issue first and leave it alone when its `pin_order` already shows the wanted state, as `gh` does; a server that sends no `pin_order` gets the request anyway. Gitea needs write access to the repository's issues for both. app.copia.io answers both `POST` and `DELETE` on the route with HTTP 404 for an issue it serves, and its issues carry no `pin_order` (found by the `integration` job of #369), so on Copia both commands report that the host does not support pinning and the issue stays as it is.

`gh issue lock`, `gh issue unlock`: `PUT /repos/{owner}/{repo}/issues/{index}/lock` with `lock_reason`, `DELETE` on the same path. `--reason/-r` takes gh's values `off_topic`, `resolved`, `spam` and `too_heated`, sent as Gitea's default lock reasons `Off-topic`, `Resolved`, `Spam` and `Too heated`; Gitea's API does not check the reason. Both read the issue's `is_locked` first. Gitea ignores a lock request for a locked issue, so `lock` fails on one, as `gh` does without a prompt; `unlock` leaves an unlocked issue alone. Gitea needs write access to the repository's issues for both. app.copia.io answers both `PUT` and `DELETE` on the route with HTTP 404 for an issue it serves (found by the `integration` job of #370), so on Copia `lock` reports that the host does not support locking and the issue stays unlocked; `unlock` finds nothing locked and sends nothing.

`gh issue status`: three listings of `GET /repos/{owner}/{repo}/issues?state=open&type=issues` for the user of `GET /user`, with `assigned_by`, `mentioned_by` and `created_by`. app.copia.io ignores `created_by` (found by the `integration` job of #376), so the author of each issue is checked on the client, and the assignees the same way; every page is read, since the share of kept issues is not known. The Gitea list has no sort parameter, so each section is sorted by `updated_at` on the client and, as in gh, shows the ten most recently updated issues and `And N more`. `--json` prints one object with gh's keys `assigned`, `mentioned` and `createdBy`; the fields are the Gitea names (`number`, `title`, `state`, `labels`, `updated_at`, `html_url`, `user`, `assignees`).

`gh issue transfer`: Gitea 1.26 has no endpoint that moves an issue to another repository.

## pr

| gh command | Parity | Copia CLI | Status |
|-----------|--------|-----------|--------|
| `gh pr list` | Full | `copia-cli pr list` | [x] |
| `gh pr create` | Full | `copia-cli pr create` | [x] |
| `gh pr view` | Full | `copia-cli pr view` | [x] |
| `gh pr merge` | Full | `copia-cli pr merge` | [x] |
| `gh pr close` | Full | `copia-cli pr close` | [x] |
| `gh pr reopen` | Full | `copia-cli pr reopen` | [x] |
| `gh pr diff` | Full | `copia-cli pr diff` | [x] |
| `gh pr checkout` | Partial | `copia-cli pr checkout` | [x] |
| `gh pr review` | Full | `copia-cli pr review` | [x] |
| `gh pr checks` | Partial | `copia-cli pr checks` | [ ] |
| `gh pr comment` | Full | `copia-cli pr comment` | [x] |
| `gh pr edit` | Full | `copia-cli pr edit` | [x] |
| `gh pr ready` | Full | `copia-cli pr ready` | [x] |
| `gh pr update-branch` | Full | `copia-cli pr update-branch` | [x] |
| `gh pr status` | Partial | `copia-cli pr status` | [x] |

`--web/-w` on `gh pr view`, `gh pr comment`, `gh pr diff`, `gh pr list` and `gh pr create`: client side, no API call and no token, opened as for the issue commands. `view` and `comment` open `/{owner}/{repo}/pulls/{index}` and `diff` opens `/{owner}/{repo}/pulls/{index}/files` (gh opens the files tab too). `list` opens `/{owner}/{repo}/pulls` with `state`; the Gitea 1.26 web list knows `open`, `closed` and `all` only, so `--state merged` opens `state=closed` with a warning. `create` opens `/{owner}/{repo}/compare/{base}...{head}` (each branch name escaped per path segment, the head defaults to the current branch, `--fill` runs first) with `expand=1`, `title` and `body`, which the compare handler reads to open and fill the new pull request form. That form takes no labels, assignees or milestone by name, so those flags print a warning; `--draft` and `--reviewer` fail with `--web` as in gh, and `--web` with `--editor` fails.

`gh pr comment`: `POST /repos/{owner}/{repo}/issues/{index}/comments`, since Gitea keeps pull request comments on the issue of the same index. `--body/-b`, `--body-file/-F` (`-` reads stdin), `--editor/-e` or `--web/-w` is required. The argument is a number, `#<n>` or a pull request URL.

`gh pr edit`: `PATCH /repos/{owner}/{repo}/pulls/{index}` for `--title/-t`, `--body/-b`, `--body-file/-F`, `--base/-B`, `--milestone/-m` and the label and assignee flags; that PATCH replaces the whole label and assignee lists, so the pull request is read first and the changed lists are sent. Gitea's pull request PATCH ignores milestone 0, so `--remove-milestone` sends `PATCH /repos/{owner}/{repo}/issues/{index}` with `milestone: 0`. `--add-reviewer` and `--remove-reviewer` use `POST` and `DELETE /repos/{owner}/{repo}/pulls/{index}/requested_reviewers` (`org/team` goes into `team_reviewers`). There is no editor, no `--add-project` and no `--remove-project`. The argument is a number, `#<n>` or a pull request URL.

`gh pr ready`: Gitea has no draft field; a pull request is a draft while its title starts with one of the `WORK_IN_PROGRESS_PREFIXES` (default `WIP:` and `[WIP]`, any case). The command reads the pull request and sends `PATCH /repos/{owner}/{repo}/pulls/{index}` with the title without the prefix; `--undo` adds `WIP: `. A pull request already in the wanted state is left alone and a closed one fails, as in `gh`. A server with other configured prefixes is not detected. The argument is a number, `#<n>` or a pull request URL.

`gh pr reopen`: `PATCH /repos/{owner}/{repo}/pulls/{index}` with `state: open`. The pull request is read first: an open one is left alone and a merged one fails, as in `gh`. `--comment/-c` posts to `/repos/{owner}/{repo}/issues/{index}/comments` before the reopen. The argument is a number, `#<n>` or a pull request URL.

`gh pr update-branch`: `POST /repos/{owner}/{repo}/pulls/{index}/update` with `style=merge`, or `style=rebase` for `--rebase`. The pull request is read first: a closed or merged one fails, since Gitea answers it with HTTP 422 without a message. Gitea answers an up-to-date branch with HTTP 500, so the command leaves the branch alone when the pull request's `merge_base` equals `base.sha` (the tip of the base branch), as `gh` does with its compare; without a `merge_base` the update is sent and the server decides. Gitea recomputes `merge_base` in the background after a push, so right after an update it can lag for a moment and the next update gets the HTTP 500. `GET /repos/{owner}/{repo}/compare/{basehead}`, which `gh` uses for this check, answers HTTP 404 on app.copia.io (found by the `integration` job of #375). HTTP 409 (merge or rebase conflict) and 422 show the server message. The argument is a number, `#<n>` or a pull request URL; `gh`'s default to the pull request of the current branch is not implemented.

`gh pr status`: one listing of `GET /repos/{owner}/{repo}/pulls?state=open&sort=recentupdate` (every page, also sorted by `updated_at` on the client) gives the pull request of the current branch (matched on `head.ref`) and the ones whose author is the user of `GET /user`. When the branch has no open pull request, the first page of `state=closed` (the 50 most recently updated) is searched, as gh also shows a closed or merged one; the list has no head filter, so older closed pull requests are not searched, which would read every page; a closed one of the default branch (`base.repo.default_branch`) is left out, as in gh. The review requests come from `GET /repos/issues/search?type=pulls&state=open&review_requested=true&owner={owner}`, kept when `repository.full_name` is this repository and shown from the open listing. Each section shows ten pull requests and `And N more`. `-R` leaves out the current branch, as in gh. `--conflict-status/-c` prints `✓ No merge conflicts` or `× Merge conflicts` from Gitea's `mergeable`, which is also false while Gitea still checks. Partial: no check or review summary line (`pr checks` is not implemented and Gitea has no review decision), and the branch is matched by name only, not through gh's push remote and `merge` ref. `--json` prints one object with gh's keys `currentBranch` (a pull request or `null`), `createdBy` and `needsReview`; the fields are the Gitea names (`number`, `title`, `state`, `html_url`, `user`, `head`, `base`, `updated_at`, `mergeable`, `draft`), and a merged pull request has the state `merged`.

## release

| gh command | Parity | Copia CLI | Status |
|-----------|--------|-----------|--------|
| `gh release list` | Full | `copia-cli release list` | [x] |
| `gh release create` | Full | `copia-cli release create` | [x] |
| `gh release view` | Full | `copia-cli release view` | [x] |
| `gh release delete` | Full | `copia-cli release delete` | [x] |
| `gh release upload` | Full | `copia-cli release upload` | [x] |
| `gh release download` | Full | `copia-cli release download` | [x] |
| `gh release edit` | Full | `copia-cli release edit` | [x] |
| `gh release delete-asset` | Full | `copia-cli release delete-asset` | [x] |

`gh release view [<tag>]`: `GET /repos/{owner}/{repo}/releases/tags/{tag}`. Without a tag it shows the newest release that is neither a draft nor a pre-release, as GitHub's latest release, from `GET /repos/{owner}/{repo}/releases?draft=false&pre-release=false` (newest first; the flags are checked on the client too, and further pages are read until one matches): Copia (0.60.0 on 2026-11-07) answers `GET /repos/{owner}/{repo}/releases/latest` with the same 404 as for a missing tag even when such a release exists. Without a terminal it prints gh's tab-separated `title`, `tag`, `draft`, `prerelease`, `author`, `created`, `published` (not for a draft) and `url` lines, one `asset` line per asset, `--` and the notes; gh's `immutable` line is left out, since Gitea has no immutable releases. On a terminal it prints the tag, `<login> released this <time ago>` (`created this` for a draft, with `Draft` or `Pre-release` in front), the notes as plain text (gh renders Markdown) and a `NAME`/`SIZE` table of the assets (gh's `Digest` column is left out, Gitea has no asset digest). `--json` takes the Gitea field names `id`, `tag_name`, `name`, `body`, `draft`, `prerelease`, `created_at`, `published_at`, `author` (`login`), `html_url`, `tarball_url`, `zipball_url`, `target_commitish` and `assets` (`id`, `name`, `size`, `download_count`, `browser_download_url`). `--web/-w` with a tag opens `/{owner}/{repo}/releases/tag/{tag}` with no API call and no token (gh looks the release up first); without a tag it looks the latest release up as above and opens its `html_url`, as gh does. A missing release fails with `release <tag> not found`, or `release not found` without a tag. `release delete` and `release upload` use the same lookup by tag, which now path-escapes the tag.

`gh release edit <tag>`: finds the release with the lookup of `release view` and sends `PATCH /repos/{owner}/{repo}/releases/{id}` with only the given fields: `--draft` and `--prerelease` (`=false` allowed) as `draft` and `prerelease`, `-t/--title` as `name`, `-n/--notes` or `-F/--notes-file` (`-` reads standard input) as `body`, `--tag` as `tag_name` and `--target` as `target_commitish`. Without one of these flags it fails with `use flags to specify properties to edit` and makes no API call. An empty title or notes (`--title ""`, `--notes ""`, an empty `--notes-file` or empty standard input) fails with `cannot clear a release's title: Copia keeps the old value` (or `notes`) before any API call: gh clears the field, but Gitea's `EditRelease` ignores an empty string and keeps the old value. An empty `--tag` or `--target` is dropped for the same reason; given alone it fails with `use flags to specify properties to edit`. It prints the release URL. gh's `--latest` and `--verify-tag` are left out (Gitea has no latest marker to set and no tag check on edit), as is `--discussion-category` (no discussions).

`gh release download [<tag>]`: finds the release with the lookup of `release view` (the latest one without a tag, where `-p/--pattern` or `-A/--archive` is required) and downloads every asset, or the ones whose name matches a `-p/--pattern` glob, from its `browser_download_url`; `-A/--archive zip|tar.gz` downloads `GET /repos/{owner}/{repo}/archive/<tag>.<format>` under the name of its `Content-Disposition` header, else `<repo>-<tag>.<format>`. `-D/--dir`, `-O/--output` (`-` is standard output), `--clobber` and `--skip-existing` and the flag checks follow gh, messages included. An asset name with a path separator or `..` is refused before any download, the file is written to a temporary file in the target directory and renamed, and a download URL that is not https on the configured host is refused without a request. Copia answers the download with a redirect to presigned storage on another host (AWS S3); only the download request follows it, https only, at most 3 hops, without `Authorization` once it leaves the configured host. The release lookup keeps the same-origin rule of every command. gh's `--allow-escape-sequences` is not offered: copia-cli writes an asset to standard output unchanged. It prints nothing on success, as gh does without a terminal.

`gh release delete-asset <tag> <asset-name>`: finds the release with the lookup of `release view`, the asset by name in its `assets`, and sends `DELETE /repos/{owner}/{repo}/releases/{id}/assets/{asset_id}`. It needs `-y/--yes`, since copia-cli has no prompts: without it it fails with `deleting asset <name> from release <tag> is irreversible; use --yes to confirm` and makes no API call. A missing asset fails with gh's `asset <name> not found in release <tag>`. On success it prints `Deleted asset <name> from release <tag>` on standard output; gh prints that line on standard error and only on a terminal.

## label

| gh command | Parity | Copia CLI | Status |
|-----------|--------|-----------|--------|
| `gh label list` | Full | `copia-cli label list` | [x] |
| `gh label create` | Full | `copia-cli label create` | [x] |
| `gh label edit` | Full | `copia-cli label edit` | [x] |
| `gh label delete` | Full | `copia-cli label delete` | [x] |
| `gh label clone` | Full | `copia-cli label clone` | [x] |

`gh label list --web/-w`: client side, no API call and no token; opens `/{owner}/{repo}/labels`.

`gh label edit <name>`: reads every page of `GET /repos/{owner}/{repo}/labels` to find the label by its exact name (Gitea compares label names case-sensitively, GitHub does not), then `PATCH /repos/{owner}/{repo}/labels/{id}` with the flags given (`--name/-n`, `--color/-c` without a leading `#`, `--description/-d`). Gitea accepts a second label of the same name, so the conflict gh reports from the API (`label with name "<new>" already exists`) is checked on the client against the listed labels. gh prints `Label "<name>" updated in <owner>/<repo>` on a terminal only; copia-cli prints it always, as its other commands print their success line.

`gh label delete <name>`: reads every page of `GET /repos/{owner}/{repo}/labels` to find the label by its exact name, then `DELETE /repos/{owner}/{repo}/labels/{id}` (gh deletes by name, which the Gitea API does not offer). Without `--yes` it fails before any API call, with or without a terminal, since copia-cli never prompts; gh's deprecated `--confirm` is not added. gh prints `Label "<name>" deleted from <owner>/<repo>` on a terminal only; copia-cli prints it always.

`gh label clone <source-repository>`: reads every page of `GET /repos/{owner}/{repo}/labels` of the source and of the destination (`-R` or the current repository), then sends `POST /repos/{owner}/{repo}/labels` with the name, color and description of each source label the destination lacks, and with `--force/-f` `PATCH /repos/{owner}/{repo}/labels/{id}` with the color and description of each one it has by exact name. Gitea accepts a second label of the same name, so existing labels are found in the list rather than from gh's `already_exists` error; labels only in the destination stay. The requests run one after another instead of gh's ten workers, and the first failed request aborts. gh prints its result on a terminal only; copia-cli prints `Cloned N labels from <source> to <destination>` on standard output always, or `! Cloned N labels of TOTAL from <source> to <destination>` on standard error when labels were skipped.

## config

| gh command | Parity | Copia CLI | Status |
|-----------|--------|-----------|--------|
| `gh config get` | Full | `copia-cli config get` | [x] |
| `gh config set` | Full | `copia-cli config set` | [x] |
| `gh config list` | Full | `copia-cli config list` | [x] |
| `gh config clear-cache` | None | n/a, copia-cli keeps no API cache | — |

Client side, no API call. The keys are `output` (`table` or `json`, default `table`), `browser`, `pager` and `editor`, stored as top-level keys of `config.yml` beside `hosts`; gh's other keys (`git_protocol`, `prompt`, `http_unix_socket` and the rest) have no copia-cli behaviour and are not accepted. gh's `--host` flag of the three commands is left out, since no setting is per host. `browse` reads `browser` between `COPIA_BROWSER` and `BROWSER`. `output` has no gh counterpart (gh's `--json` always names its fields): `COPIA_OUTPUT`, then `output`, then `table` choose it, and with `json` every command that takes `--json` prints all its fields without `--json`, where `--jq` and `--template` work too; an explicit `--json` field list wins. `pager` works as in gh: `COPIA_PAGER` (gh: `GH_PAGER`), then `pager`, then `PAGER` choose it, and the list and view commands and `pr diff` use it when standard output is a terminal; the command is split at whitespace without quoting, where gh splits it like a shell. `editor` is used by `--editor/-e` of `issue create`, `pr create`, `issue comment` and `pr comment`: `COPIA_EDITOR` (gh: `GH_EDITOR`), then `editor`, then `VISUAL`, then `EDITOR` choose it, and the command is split like a shell and run without one, as in gh. Unlike gh there is no default editor (gh falls back to `nano` or `notepad`): with none set `--editor` fails, and the editor opens only on `--editor`, since copia-cli has no interactive prompts.

## alias

| gh command | Parity | Copia CLI | Status |
|-----------|--------|-----------|--------|
| `gh alias set` | Full | `copia-cli alias set` | [x] |
| `gh alias list` | Full | `copia-cli alias list` | [x] |
| `gh alias delete` | Full | `copia-cli alias delete` | [x] |
| `gh alias import` | Full | `copia-cli alias import` | [x] |

Client side, no API call. Aliases are stored under the top-level key `aliases` of `config.yml` and expanded before the command runs when the alias is the first argument, as in gh: `$1`, `$2` and so on take the arguments after the alias, otherwise they are appended, and an expansion that starts with `!` (or was set with `--shell/-s`) runs through `sh -c` with the arguments as `$1`, `$2`. Differences from gh: an argument fills a placeholder after the expansion is split into words, so it is never split again (gh replaces the placeholders in the text and splits afterwards); an alias name may not be empty, start with `-` or contain blanks, and the expansion of an alias that is no shell alias must start with a copia-cli command (gh also accepts an extension or another alias, which copia-cli does not have or expand); a built-in command also wins over an alias of the same name that was written into the config file by hand; `alias list` and `alias set`, `delete` and `import` print their messages whether or not the output is a terminal, and `alias import` exits with an error when it refused an alias, after it imported the others (gh exits 0).

## Other implemented commands

| Command | Status |
|---------|--------|
| `copia-cli api` | [x] |
| `copia-cli search repos` (`--web/-w` opens `/explore/repos` with `q`, and `--sort`/`--order` as the explore sort names of Gitea 1.26, for example `moststars`) | [x] |
| `copia-cli search issues` (`--web/-w` opens `/{owner}/{repo}/issues` with `q` and `state`, default `all`; `--label` is not applied there) | [x] |
| `copia-cli search prs` (`gh search prs`: query, `--state`, `--label`, `--author`, `--assignee`, `--owner`, `-R`, `--limit`; `GET /repos/issues/search?type=pulls`, `--assignee` only for the authenticated user, `-R` filters on the client side, labels match any name; app.copia.io ignores `created_by`, found by the `integration` job of #376, so `--author` is also checked on each result. `--web/-w`: Copia has no web search across repositories, so it opens the pull request dashboard `/pulls`, or `/org/{owner}/pulls` with `--owner` (organization members only), with `q`, `state` (the dashboard knows open and closed, open by default), `poster` and `assignee` as logins and `type=created_by` or `type=assigned` for `@me`; with `-R` it opens `/{owner}/{repo}/pulls`, which takes the assignee only by ID. `--label` is not applied in the browser) | [x] |
| `copia-cli org list` | [x] |
| `copia-cli org view` | [x] |
| `copia-cli notification list` | [x] |
| `copia-cli notification read` | [x] |
| `copia-cli browse` (`gh browse`: repository, issue or PR number, commit SHA (`/commit/{sha}`), `<path>`, `<path>:<line>` and `<path>:<start>-<end>` (`/src/branch/{branch}/{path}`, with a line `?display=source#L{n}` or `#L{n}-L{m}`, Gitea's counterpart of gh's `?plain=1`), `--branch/-b`, `--commit/-c` (`/src/commit/{sha}/{path}`; without a SHA the last commit of the current branch, of the default branch with `-R`, from `GET /repos/{owner}/{repo}/branches/{branch}`), `--blame` (`/blame/branch/...` or `/blame/commit/...`), `--settings/-s`, `--wiki/-w`, `--releases/-r`, `--actions/-a`, `--projects/-p`, `--no-browser/-n`; a path without `--branch` or `--commit` reads the default branch with `GET /repos/{owner}/{repo}`, everything else makes no API call. Conflicting flags fail with gh's messages before anything opens. Unlike gh, `--no-browser` does not check that the repository exists) | [x] |
| `copia-cli status` (`gh status`, Partial: the sections Assigned Issues, Assigned Pull Requests, Review Requests and Mentions come from `GET /repos/issues/search?state=open` with `assigned=true` (split on `pull_request`), `review_requested=true&type=pulls` and `mentioned=true`, the first 25 results of each sorted by `updated_at` and five shown, as gh reads 25 search results and shows five. gh takes its mentions from the notifications with the reason `mention`; Gitea's notifications carry no reason, so the search filter is used. Repository Activity is the ten newest of `GET /notifications?all=true` with the subject type (`issue`, `PR`, `commit`, `repository`) instead of gh's `new issue`, `new PR` and `comment on` from `/users/{user}/received_events`, which Gitea 1.26 does not have. `--org/-o` is sent as the search's `owner` and checked on every result and notification, `--exclude/-e` is applied on the client, both regardless of case; an `--exclude` value that is not `owner/repo` fails. The layout is gh's: two half-width columns separated by `│` at the terminal width (80 when not a terminal), without colour) | [x] |
| `copia-cli completion` | [x] |

## Other gh commands (not yet implemented)

| gh command group | Parity | Notes |
|-----------------|--------|-------|
| `gh ssh-key` | Full | `GET/POST /user/keys` |
| `gh gpg-key` | Full | `GET/POST /user/gpg_keys` |
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
| Implemented | 74 |
| Not yet implemented (Full parity) | 4 |
| Not yet implemented (Partial) | 6 |
| No equivalent | 12 |
| **Total implementable** | **84** |

Counted from the rows above: every `[x]` row, and every other row by its Parity value. The Parity values follow the Gitea 1.26 API (`release/v1.26` swagger); `TestAPI_Version` in `test/integration/api_test.go` logs what Copia's `GET /version` reports in the PR `integration` job. On 2026-11-04 that was `0.59.0`, Copia's own version number, not a Gitea release, so the API does not show which Gitea version Copia is built on; a command's integration test shows whether Copia serves its endpoint.
