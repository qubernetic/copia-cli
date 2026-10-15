# Copia / Gitea API Reference

Developer reference for the REST API that powers the Copia CLI.

## Base URL

```
https://app.copia.io/api/v1/{endpoint}
```

All endpoints require authentication. No anonymous access.

## Authentication

| Method | Header | Notes |
|--------|--------|-------|
| **API Token** (recommended) | `Authorization: token <key>` | Personal Access Token from Copia Settings > Applications |
| Basic Auth | `Authorization: Basic <base64>` | Username + password |
| Basic Auth + 2FA | + `X-GITEA-OTP: <code>` | Required when 2FA is enabled |

**Token generation via API:**
```
POST /api/v1/users/{username}/tokens
Authorization: Basic <base64>
Content-Type: application/json

{"name": "copia-cli"}
```

**Removed on Copia since 2026-05-01:** Query parameter tokens (`?access_token=` and `?token=`) return `401 Unauthorized`. Header auth is the only supported method, and copia-cli only sends the token in the `Authorization` header. The unit-test HTTP mock (`pkg/httpmock`) rejects any request with a `token` or `access_token` query parameter, and the integration test `TestAuth_QueryParamToken_Rejected` checks the live behaviour.

## Endpoint Overview

470 endpoints across 9 categories in the Gitea 1.26 API:

| Category | Count | Key Operations |
|----------|-------|----------------|
| **Repository** | 197 | CRUD repos, branches, commits, contents, files, hooks, keys, pulls, releases, tags, topics, collaborators, forks |
| **User** | 76 | Profile, emails, followers, GPG/SSH keys, repos, starred, watched, tokens |
| **Issue** | 69 | CRUD issues, comments, labels, milestones, reactions, attachments, time tracking |
| **Organization** | 66 | CRUD orgs, teams, members, repos, labels, hooks, secrets |
| **Admin** | 32 | User/org management, cron jobs, Action runners |
| **Miscellaneous** | 12 | Version, signing key, markdown render, templates |
| **Package** | 8 | List/get/delete packages |
| **Notification** | 7 | List/read/mark notifications |
| **Settings** | 4 | API/attachment/repo/UI settings |

## Core Endpoints

### Repository

```
GET    /repos/{owner}/{repo}                     # View repo
POST   /user/repos                               # Create repo (personal)
POST   /orgs/{org}/repos                         # Create repo (org)
DELETE /repos/{owner}/{repo}                      # Delete repo
POST   /repos/{owner}/{repo}/forks               # Fork repo
GET    /repos/search?q={query}                    # Search repos

GET    /repos/{owner}/{repo}/branches             # List branches
POST   /repos/{owner}/{repo}/branches             # Create branch
DELETE /repos/{owner}/{repo}/branches/{branch}    # Delete branch

GET    /repos/{owner}/{repo}/contents/{path}      # Read file
POST   /repos/{owner}/{repo}/contents/{path}      # Create file
PUT    /repos/{owner}/{repo}/contents/{path}      # Update file
DELETE /repos/{owner}/{repo}/contents/{path}      # Delete file
GET    /repos/{owner}/{repo}/raw/{path}           # Raw file content

GET    /repos/{owner}/{repo}/commits              # List commits
GET    /repos/{owner}/{repo}/git/refs             # Git references
GET    /repos/{owner}/{repo}/git/trees/{sha}      # Git trees
GET    /repos/{owner}/{repo}/topics               # List topics
```

### Pull Requests

```
GET    /repos/{owner}/{repo}/pulls                # List PRs
POST   /repos/{owner}/{repo}/pulls                # Create PR
GET    /repos/{owner}/{repo}/pulls/{index}        # View PR
PATCH  /repos/{owner}/{repo}/pulls/{index}        # Update PR (title, body, state)
POST   /repos/{owner}/{repo}/pulls/{index}/merge  # Merge PR

GET    /repos/{owner}/{repo}/pulls/{index}.diff   # PR diff
GET    /repos/{owner}/{repo}/pulls/{index}/files  # Changed files
GET    /repos/{owner}/{repo}/pulls/{index}/commits # PR commits

POST   /repos/{owner}/{repo}/pulls/{index}/reviews         # Submit review
GET    /repos/{owner}/{repo}/pulls/{index}/reviews          # List reviews
POST   /repos/{owner}/{repo}/pulls/{index}/reviews/{id}     # Submit pending review

POST   /repos/{owner}/{repo}/pulls/{index}/requested_reviewers  # Request review
```

### Issues

```
GET    /repos/{owner}/{repo}/issues               # List issues
POST   /repos/{owner}/{repo}/issues               # Create issue
GET    /repos/{owner}/{repo}/issues/{index}        # View issue
PATCH  /repos/{owner}/{repo}/issues/{index}        # Update issue

GET    /repos/{owner}/{repo}/issues/{index}/comments  # List comments
POST   /repos/{owner}/{repo}/issues/{index}/comments  # Add comment

POST   /repos/{owner}/{repo}/issues/{index}/labels    # Add labels
DELETE /repos/{owner}/{repo}/issues/{index}/labels/{id} # Remove label

GET    /repos/{owner}/{repo}/issues/{index}/reactions # List reactions
POST   /repos/{owner}/{repo}/issues/{index}/reactions # Add reaction
```

### Labels

```
GET    /repos/{owner}/{repo}/labels               # Repo labels
POST   /repos/{owner}/{repo}/labels               # Create repo label
GET    /orgs/{org}/labels                          # Org labels
POST   /orgs/{org}/labels                          # Create org label
PATCH  /repos/{owner}/{repo}/labels/{id}           # Update label
DELETE /repos/{owner}/{repo}/labels/{id}           # Delete label
```

### Milestones

```
GET    /repos/{owner}/{repo}/milestones            # List milestones
POST   /repos/{owner}/{repo}/milestones            # Create milestone
PATCH  /repos/{owner}/{repo}/milestones/{id}       # Update milestone
DELETE /repos/{owner}/{repo}/milestones/{id}       # Delete milestone
```

### Releases

```
GET    /repos/{owner}/{repo}/releases              # List releases
POST   /repos/{owner}/{repo}/releases              # Create release
GET    /repos/{owner}/{repo}/releases/{id}         # View release
PATCH  /repos/{owner}/{repo}/releases/{id}         # Update release
DELETE /repos/{owner}/{repo}/releases/{id}         # Delete release
GET    /repos/{owner}/{repo}/releases/tags/{tag}   # Get by tag

POST   /repos/{owner}/{repo}/releases/{id}/assets  # Upload asset
GET    /repos/{owner}/{repo}/releases/{id}/assets   # List assets
```

### Organization

```
GET    /orgs/{org}                                 # View org
GET    /orgs/{org}/repos                           # List org repos
GET    /orgs/{org}/members                         # List members
GET    /orgs/{org}/teams                           # List teams
GET    /orgs/{org}/labels                          # Org labels
```

### User

```
GET    /user                                       # Authenticated user
GET    /users/{username}                           # View user
GET    /user/repos                                 # List own repos
GET    /user/orgs                                  # List own orgs
GET    /user/keys                                  # SSH keys
POST   /user/keys                                  # Add SSH key
GET    /user/gpg_keys                              # GPG keys
```

### Notifications

```
GET    /notifications                              # List notifications
PUT    /notifications                              # Mark all read
GET    /repos/{owner}/{repo}/notifications         # Repo notifications
PUT    /repos/{owner}/{repo}/notifications         # Mark repo notifications read
```

## Request/Response Patterns

### Create Issue (example)

```bash
curl -X POST \
  -H "Authorization: token YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "fix sensor mapping",
    "body": "## Problem\nSensor I/O mapping is incorrect.\n\n## Expected\nCorrect mapping.",
    "labels": [7126, 9624]
  }' \
  "https://app.copia.io/api/v1/repos/my-org/my-repo/issues"
```

### Pagination

All list endpoints support pagination:
```
?page=1&limit=50
```

Default limit varies by endpoint (typically 50). Response headers include:
- `x-total-count` — total number of items
- `Link` — RFC 5988 pagination links

### Error Responses

| Status | Meaning |
|--------|---------|
| 403 | Authentication required or insufficient permissions |
| 404 | Resource not found (or no access) |
| 409 | Conflict (e.g., branch already exists) |
| 422 | Validation error (check response body for details) |

## Copia-Specific Notes

- **PLC binary rendering** — Copia adds visual diff capabilities for PLC files. These may use custom API endpoints not documented in standard Gitea API
- **DeviceLink** — Automatic PLC backups from physical devices. Likely separate API surface
- **Swagger spec** — Available at `https://app.copia.io/swagger.v1.json` but requires browser auth (not API token), so custom Copia endpoints cannot be enumerated via API token alone
- **Rate limiting** — Instance-configurable, no published limits for Copia

## Sources

- [Gitea API Usage Documentation](https://docs.gitea.com/development/api-usage)
- [Gitea API Reference (Swagger)](https://docs.gitea.com/api/)
- [Gitea Demo Swagger UI](https://demo.gitea.com/api/swagger)
