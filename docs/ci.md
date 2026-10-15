# CI credentials

Most workflows use the job's `GITHUB_TOKEN`. One workflow needs more:
`bump-go`, which opens a pull request each time a new Go version comes out.

GitHub does not start workflows for pushes or pull requests made with
`GITHUB_TOKEN`. A bump-go PR opened that way gets no `test` check. So
`bump-go` mints a short-lived token from a GitHub App and opens the PR with
that token. The App token does start `ci.yml`.

## The bump-go GitHub App

| | |
|---|---|
| Name | `copia-cli-bump-go` |
| Owner | the `qubernetic` organisation |
| Installed on | `qubernetic/copia-cli` only ("Only select repositories") |
| Repository permissions | Contents: read and write; Pull requests: read and write; Metadata: read |
| Organisation and account permissions | none |
| Webhook | off (clear "Active") |
| Where it can be installed | only on this account |

The App has no other permissions or installations.

### Creating the App

1. Go to the `qubernetic` organisation's **Settings > Developer settings > GitHub Apps > New GitHub App**.
2. Name: `copia-cli-bump-go`. Homepage URL: `https://github.com/qubernetic/copia-cli`.
3. Under **Webhook**, clear **Active**.
4. Set the repository permissions from the table above. Leave everything else at "No access".
5. **Where can this GitHub App be installed?** Only on this account.
6. Create the App. Note its **App ID** on the App's settings page.
7. Under **Private keys**, select **Generate a private key**. The browser downloads a `.pem` file.
8. **Install App** > `qubernetic` > **Only select repositories** > `copia-cli`.

## Secrets

Two repository secrets on `qubernetic/copia-cli`
(**Settings > Secrets and variables > Actions**):

| Secret | Value |
|---|---|
| `BUMP_GO_APP_ID` | The App ID from the App's settings page (the Client ID works too) |
| `BUMP_GO_APP_PRIVATE_KEY` | The full contents of the `.pem` file, including the `BEGIN` and `END` lines |

```bash
gh secret set BUMP_GO_APP_ID --repo qubernetic/copia-cli --body "<app id>"
gh secret set BUMP_GO_APP_PRIVATE_KEY --repo qubernetic/copia-cli < copia-cli-bump-go.<date>.private-key.pem
```

Delete the `.pem` file afterwards.

Only `.github/workflows/bump-go.yml` reads these secrets, in its
"Create App token" step (`actions/create-github-app-token`). That step asks
for a token for `qubernetic/copia-cli` only, with Contents and Pull requests
write. The workflow's own `GITHUB_TOKEN` only reads; it checks out the code.

## Rotation

- **Installation tokens** expire after one hour, and `actions/create-github-app-token` revokes them when the job ends. Nothing to rotate.
- **The private key** does not expire. Rotate it when someone with access to it leaves, when you suspect a leak, and once a year:
  1. On the App's settings page, select **Generate a private key**.
  2. Update `BUMP_GO_APP_PRIVATE_KEY` with the new `.pem` file (see above).
  3. Run `bump-go` once: `gh workflow run bump-go.yml --repo qubernetic/copia-cli`, then check that the "Create App token" step passed (`gh run list --workflow bump-go.yml --limit 1`, then `gh run view <id>`). If Go is already current, the step is skipped; the next real bump checks the key instead.
  4. Delete the old key on the App's settings page.

## Checking that it works

The next bump-go PR shows the `test` check:

```bash
gh pr list --repo qubernetic/copia-cli --search "bump Go in:title" --limit 1
gh pr checks <number> --repo qubernetic/copia-cli
```

`gh pr checks` lists `test` (and the rest of `CI`). The PR's author is
`copia-cli-bump-go[bot]`, not `github-actions[bot]`. If the list is empty,
the PR was opened with `GITHUB_TOKEN`: check that both secrets exist
(`gh secret list --repo qubernetic/copia-cli`) and read the bump-go run log.
