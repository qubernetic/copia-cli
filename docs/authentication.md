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
```

**Security considerations:**
- File permissions should be `600` (owner read/write only)
- On Windows, stored in `%USERPROFILE%\.config\copia\config.yml`
- Token can also be set via `COPIA_TOKEN` environment variable (takes precedence)
- Token can also be passed per-command via `--token` flag (takes highest precedence)

### Check the Auth Status

`copia-cli auth status` lists every host in the config file with its user, its token and whether Copia accepts the token. `--host` limits the output to one host.

`auth status` reads only the config file and the `--host` flag. It does not read `COPIA_HOST` or `COPIA_TOKEN`, so when either is set, the other commands may use a different host or token than the one `auth status` reports on.

```console
$ copia-cli auth status
app.copia.io
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
3. Config file (`~/.config/copia/config.yml`)

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

The CLI should support multiple Copia instances (e.g., on-prem vs cloud):

```yaml
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
3. Git remote URL of the current repo (auto-detect)
4. First entry in config file
