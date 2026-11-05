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

The CLI will store tokens in `~/.config/copia/config.yml`:

```yaml
hosts:
  app.copia.io:
    token: <encrypted-or-plaintext>
    user: <username>
    default_org: my-org
output: json
browser: firefox --new-tab
aliases:
  co: pr checkout
  bugs: issue list --label $1
```

`default_host` names the host the commands use when neither `--host` nor `COPIA_HOST` is given. `auth login` sets it to the host it logs in to, and `auth logout` of that host moves it to the alphabetically first remaining host, or removes it when none remains. When it is unset (a config of an older version) or names a host that is no longer in `hosts`, the alphabetically first host is the default.

The other top-level keys beside `hosts` are the settings of `copia-cli config` (`output`, `browser`, `pager`, `editor`); `config set` changes one of them and keeps `hosts` and the tokens as they are. Every save keeps the keys copia-cli does not know, at the top level and under a host (such as `default_org` above), and writes them back unchanged.

`output: json` makes every command that takes `--json` print all its fields as JSON when `--json` is not given, so `--jq` and `--template` work alone. `COPIA_OUTPUT` (`table` or `json`) overrides the key for one call, and an explicit `--json` field list overrides both.

`pager` names the program the list and view commands and `pr diff` send their output to when standard output is a terminal, for example `less -R`. `COPIA_PAGER` overrides it (set but empty, it turns the pager off), and `PAGER` is used when neither is set. The value is split into words at whitespace and run without a shell.

`editor` names the program that `--editor/-e` of `issue create`, `pr create`, `issue comment` and `pr comment` opens on a temporary file, for example `vim` or `code --wait`. `COPIA_EDITOR` overrides it, and `VISUAL`, then `EDITOR` are used when neither is set; with none of them `--editor` fails. The value is split into words like a shell does (quotes and backslashes group words, nothing is expanded) and run without a shell, with the file name as the last argument. The temporary file (`copia-*.md`, mode `600`) is removed afterwards.

`aliases` maps an alias name to its expansion; `copia-cli alias set`, `delete` and `import` change it and keep `hosts` and the tokens as they are, and `alias list` prints only the aliases. An alias runs as the expanded command, so `copia-cli co 7` runs `copia-cli pr checkout 7`; an expansion that starts with `!` runs through `sh -c` as the current user. An alias may not have the name of a copia-cli command, and a copia-cli command wins over an alias of the same name added by hand.

**Security considerations:**
- File permissions are `600` (owner read/write only): every save writes a new `600` file to a temporary file beside it and renames it over the old one, so a failed save leaves the old file as it was. A save refuses a config path that is not a regular file (a symlink, for example) and a file the user cannot write; a config directory that group or others can write is set to `700`, and the save fails if that is not possible
- On Windows, stored in `%USERPROFILE%\.config\copia\config.yml`
- Token can also be set via `COPIA_TOKEN` environment variable (takes precedence)
- Token can also be passed per-command via `--token` flag (takes highest precedence)

### Check the Auth Status

`copia-cli auth status` lists every host in the config file in alphabetical order, the default host marked with `(default)`, with its user, its token and whether Copia accepts the token. `--host` limits the output to one host.

`auth status` reads only the config file and the `--host` flag. It does not read `COPIA_HOST` or `COPIA_TOKEN`, so when either is set, the other commands may use a different host or token than the one `auth status` reports on.

```console
$ copia-cli auth status
app.copia.io (default)
  User: john
  Token: ****************************************
  Token valid
```

The token is masked by default: one asterisk per character. Copia tokens carry no prefix, so nothing of the token stays visible. `--show-token` prints the full token instead:

```bash
copia-cli auth status --show-token
copia-cli auth status --host app.copia.io --show-token
```

**The output then contains the secret.** Do not run it where the terminal is recorded or shared, or in a CI log. The flag changes the output only; the config file is not touched. When standard output is not a terminal (a pipe, a redirect or a CI log), `auth status --show-token` still prints the token and writes one warning line to standard error.

## Auth Precedence

1. `--token` flag (per-command)
2. `COPIA_TOKEN` environment variable
3. The stored token of the active host in the config file (`~/.config/copia/config.yml`)

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
    token: abc123
    user: cbiro
    default_org: my-org
  copia.internal.example.com:
    token: xyz789
    user: cbiro
    default_org: InternalTeam
```

The active instance is determined by:
1. `--host` flag
2. `COPIA_HOST` environment variable
3. The config key `default_host`, when it names a host in `hosts`
4. The alphabetically first host in the config file

The repository a command works on is determined by:
1. `-R/--repo` flag
2. The git remote marked with `copia-cli repo set-default` (git config key `remote.<name>.copia-resolved`)
3. The git remote `origin`
