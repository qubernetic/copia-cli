# Authentication

Copia requires authentication for **all** API calls. There is no anonymous access.

## Token-Based Auth (Recommended)

### Generate a Token

1. Log in to Copia web UI
2. Go to **Settings > Applications**
3. Under "Manage Access Tokens", enter a name (e.g., `copia-cli`)
4. Click **Generate Token**
5. Copy the token immediately — it is shown only once

### Use the Token

```bash
# CLI
copia-cli auth login --token <your-token>

# Direct API call
curl -H "Authorization: token <your-token>" \
  https://app.copia.io/api/v1/user
```

The `Authorization: token <your-token>` header is the only supported way to send a token on Copia since 2026-05-01. Query parameter tokens (`?token=` and `?access_token=`) are ignored, so the call is anonymous and Copia returns `403 Forbidden`. copia-cli only uses the header.

copia-cli follows an HTTP redirect only when it keeps the scheme, host and port of the request; a redirect to another origin (another host, another port, or `https` to `http`) fails the command, so the header is never sent anywhere but the host the token belongs to.

### Token Storage

`auth login` stores the token in the OS keyring: the macOS Keychain, the Windows Credential Manager, or the Secret Service on Linux (GNOME Keyring, KWallet). The entry has the service `copia-cli:<host>` and the login as the account, so every account of a host has its own entry. The config file then keeps the host and the account, but no token.

When the keyring is not available (no Secret Service running, a headless server, a container, a CI runner) or does not answer within three seconds (a locked keyring waiting for an unlock prompt), `auth login` stores the token in plain text in the config file instead and prints a warning on standard error that names the config file; the warning never contains the token. `--insecure-storage` stores the token in the config file on purpose, without trying the keyring:

```bash
copia-cli auth login --token <your-token> --insecure-storage
```

The snap has no `password-manager-service` plug, so the snap can never reach the keyring and always stores the token in the config file, with the warning.

The commands read the stored token from the config file first, then from the keyring, so a token stored in the config file by an older version keeps working. There is no automatic migration: the next `auth login` on that host moves the token to the keyring and removes it from the config file. `auth logout` deletes the token from both places; when the keyring fails it still removes the account from the config file and prints a warning, so check the keyring by hand then.

### Several Accounts on One Host

`auth login` with the token of another account on the same host adds that account and makes it the host's active account; the accounts logged in before stay stored, each with its own token. Logging in again with an account the host already has updates its token and makes it active again. The commands always use the active account's token.

`auth logout` removes the active account of the host, `auth logout --user <login>` (`-u`) removes the named one. When the active account is removed, the alphabetically first remaining account becomes active and `Switched active account for <host> to <login>` is printed; when the last account of a host is removed, the host is removed from the config file.

```bash
copia-cli auth logout --host app.copia.io --user john
```

`auth switch` makes another stored account active and its host the default host, then prints `Switched active account for <host> to <login>`. `--host` and `--user` (`-u`) name the account. `--host` alone is enough when the host has one account (the host becomes the default host) or two (the other one becomes active). Without flags it switches to the other account when exactly two accounts are stored in total, on one host or on two. Otherwise it fails and lists the stored accounts, one `<host> <login>` per line, so you can pick one with `--host` and `--user`; there is no prompt. An unknown host or login fails and leaves the config file as it was. No token is read or moved. `COPIA_TOKEN` applies only with `COPIA_HOST` (or `--host`), so a switch never sends it to the new default host; when it is set, a warning says so. A host entry of the old shape with a token but no login cannot be switched on; run `auth login` for that host first.

```bash
copia-cli auth switch
copia-cli auth switch --host app.copia.io --user john
```

The config file is `~/.config/copia/config.yml`:

```yaml
default_host: app.copia.io
hosts:
  app.copia.io:
    user: <the active account's login>
    users:
      <login>:
        token: <plain text, only when stored in the config file>
      <other login>:
    default_org: my-org
output: json
browser: firefox --new-tab
aliases:
  co: pr checkout
  bugs: issue list --label $1
```

A host keeps several accounts, as in gh: `users` holds every account by its login (with a `token` key only when that token is stored in the config file), and `user` names the active account, whose token the commands use. A config of an older version, with `token` and `user` directly under the host, is read as one account and written in the new shape on the next save.

`default_host` names the host the commands use when neither `--host` nor `COPIA_HOST` is given. `auth login` sets it to the host it logs in to, `auth switch` to the host of the account it switches to, and `auth logout` of that host moves it to the alphabetically first remaining host, or removes it when none remains. When it is unset (a config of an older version) or names a host that is no longer in `hosts`, the alphabetically first host is the default.

The other top-level keys beside `hosts` are the settings of `copia-cli config` (`output`, `browser`, `pager`, `editor`); `config set` changes one of them and keeps `hosts` and the tokens as they are. Every save keeps the keys copia-cli does not know, at the top level and under a host (such as `default_org` above), and writes them back unchanged.

`output: json` makes every command that takes `--json` print all its fields as JSON when `--json` is not given, so `--jq` and `--template` work alone. `COPIA_OUTPUT` (`table` or `json`) overrides the key for one call, and an explicit `--json` field list overrides both.

`pager` names the program the list and view commands and `pr diff` send their output to when standard output is a terminal, for example `less -R`. `COPIA_PAGER` overrides it (set but empty, it turns the pager off), and `PAGER` is used when neither is set. The value is split into words at whitespace and run without a shell.

`editor` names the program that `--editor/-e` of `issue create`, `pr create`, `issue comment` and `pr comment` opens on a temporary file, for example `vim` or `code --wait`. `COPIA_EDITOR` overrides it, and `VISUAL`, then `EDITOR` are used when neither is set; with none of them `--editor` fails. The value is split into words like a shell does (quotes and backslashes group words, nothing is expanded) and run without a shell, with the file name as the last argument. The temporary file (`copia-*.md`, mode `600`) is removed afterwards.

`aliases` maps an alias name to its expansion; `copia-cli alias set`, `delete` and `import` change it and keep `hosts` and the tokens as they are, and `alias list` prints only the aliases. An alias runs as the expanded command, so `copia-cli co 7` runs `copia-cli pr checkout 7`; an expansion that starts with `!` runs through `sh -c` as the current user. An alias may not have the name of a copia-cli command, and a copia-cli command wins over an alias of the same name added by hand.

**Security considerations:**
- File permissions are `600` (owner read/write only): every save writes a new `600` file to a temporary file beside it and renames it over the old one, so a failed save leaves the old file as it was. A save refuses a config path that is not a regular file (a symlink, for example) and a file the user cannot write; a config directory that group or others can write is set to `700`, and the save fails if that is not possible
- On Windows, stored in `%USERPROFILE%\.config\copia\config.yml`
- Token can also be set via `COPIA_TOKEN` environment variable, together with `COPIA_HOST` (or `--host`) naming the host it belongs to; without them it is ignored with a warning (see [Auth Precedence](#auth-precedence))
- Token can also be passed per-command via `--token` flag (takes highest precedence)

### Check the Auth Status

`copia-cli auth status` lists every host in the config file in alphabetical order, the default host marked with `(default)`, and under each host every account, the active one first, with its user, `Active account: true` or `false`, its token, where the token is stored (`keyring` or `config file`) and whether Copia accepts the token. A host without a stored token shows `Token: none` and `No token stored`, and a keyring that fails shows `Token: none` and the error. `--host` limits the output to one host.

`auth status` reads only the stored tokens (config file and keyring) and the `--host` flag. It does not read `COPIA_HOST` or `COPIA_TOKEN`, so when either is set, the other commands may use a different host or token than the one `auth status` reports on.

```console
$ copia-cli auth status
app.copia.io (default)
  User: john
  Active account: true
  Token: **************************************** (keyring)
  Token valid
  User: jane
  Active account: false
  Token: **************************************** (keyring)
  Token valid
```

The token is masked by default: one asterisk per character. Copia tokens carry no prefix, so nothing of the token stays visible. `--show-token` prints the full token instead:

```bash
copia-cli auth status --show-token
copia-cli auth status --host app.copia.io --show-token
```

**The output then contains the secret.** Do not run it where the terminal is recorded or shared, or in a CI log. The flag changes the output only; neither the config file nor the keyring is touched. When standard output is not a terminal (a pipe, a redirect or a CI log), `auth status --show-token` still prints the token and writes one warning line to standard error.

### Use the Token with git

`copia-cli auth git-credential` is a hidden command that git runs as a [credential helper](https://git-scm.com/docs/gitcredentials), as `gh auth git-credential` does. Configure it for each Copia host; the empty first entry drops the other helpers for that host, so git stores the token nowhere else:

```bash
git config --global credential.https://app.copia.io.helper ''
git config --global --add credential.https://app.copia.io.helper '!copia-cli auth git-credential'
```

For `get` it reads git's `key=value` lines from standard input. When `protocol` is `https` and `host` is a configured host, it prints `username=<login>` and `password=<token>` of the host's active account, the token read from the config file or the OS keyring as for the other commands. The host matches regardless of case, and its port must match: a missing port is 443, so `copia.example.com:8443` matches only a request for port 8443. A path in the request is ignored. A `username` in the request (from a URL such as `https://jane@app.copia.io/...`) must be the active account's login. Anything else (`http`, an unknown host, another port, another user name, a host without a stored token) prints nothing and exits 0, so git falls back to its next helper or prompt. `store` and `erase` do nothing, since the token stays where `auth login` stored it.

`--token`, `COPIA_TOKEN` and `COPIA_HOST` do not apply: git asks for each host separately, and the answer is always the stored token of that host. A stored token or login with a character outside printable ASCII, such as a line break, is refused with an error that names the host, never the token.

## Auth Precedence

The host comes from `--host`, then `COPIA_HOST`, then the config's default host (`default_host`, or the alphabetically first host). The token comes from:

1. `--token` flag (per-command)
2. `COPIA_TOKEN` environment variable, only when the host came from `--host` or `COPIA_HOST`
3. The stored token of the active account of the active host: the config file (`~/.config/copia/config.yml`) first, then the OS keyring

`COPIA_TOKEN` is bound to the host you name, so it never goes to a host the config picks: `auth logout`, `auth switch`, `auth login` and the alphabetical fallback all move the default host. When `COPIA_TOKEN` is set but the host comes from the config, the commands use the stored token and print once on standard error:

```text
warning: COPIA_TOKEN is ignored because no host is named; set COPIA_HOST (or --host) to the host the token belongs to
```

When that host has no stored token, the `no token configured` error carries the same hint. The token value appears in no output or error.

`auth git-credential` (see [Use the Token with git](#use-the-token-with-git)) follows none of this: it answers git for the host git asks about, with that host's stored token.

In CI, set both variables:

```bash
export COPIA_HOST=app.copia.io
export COPIA_TOKEN="$COPIA_CI_TOKEN"
copia-cli repo list
```

## Alternative Auth Methods

### Basic Auth

```bash
curl -u "username:password" https://app.copia.io/api/v1/user
```

If 2FA is enabled, add the TOTP header:
```bash
curl -u "username:password" \
  -H "X-GITEA-OTP: 123456" \
  https://app.copia.io/api/v1/user
```

### Generate Token via API (bootstrapping)

```bash
curl -u "username:password" \
  -X POST \
  -H "Content-Type: application/json" \
  -d '{"name":"copia-cli"}' \
  https://app.copia.io/api/v1/users/{username}/tokens
```

Response:
```json
{
  "id": 1,
  "name": "copia-cli",
  "sha1": "e00321...",
  "token_last_eight": "0e1907e5"
}
```

## Multi-Instance Support

The config file holds several Copia instances (e.g., on-prem vs cloud):

```yaml
default_host: app.copia.io
hosts:
  app.copia.io:
    user: cbiro
    users:
      cbiro:
        token: abc123
      ci-bot:
        token: def456
    default_org: my-org
  copia.internal.example.com:
    user: cbiro
    users:
      cbiro:
        token: xyz789
    default_org: InternalTeam
```

The tokens above are stored in the config file (`--insecure-storage` or no keyring); a token in the keyring has its own entry per host and account (service `copia-cli:<host>`, account = the login) and no `token` key here.

The active instance is determined by:
1. `--host` flag
2. `COPIA_HOST` environment variable
3. The config key `default_host`, when it names a host in `hosts`
4. The alphabetically first host in the config file

The repository a command works on is determined by:
1. `-R/--repo` flag
2. The git remote marked with `copia-cli repo set-default` (git config key `remote.<name>.copia-resolved`)
3. The git remote `origin`
