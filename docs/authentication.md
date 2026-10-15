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

The `Authorization: token <your-token>` header is the only supported way to send a token on Copia since 2026-05-01. Query parameter tokens (`?token=` and `?access_token=`) are rejected with `401 Unauthorized`. copia-cli only uses the header.

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
