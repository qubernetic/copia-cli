# Quick start

> Install copia-cli, sign in once, then clone a repository, file an issue and open a pull request from the terminal.

The examples use the organization `my-org` and the repository `my-repo` on `app.copia.io`. Put in your own names.

## 1. Install

=== "macOS and Linux"

    ```bash
    brew install qubernetic/tap/copia-cli
    ```

=== "Windows"

    ```powershell
    winget install Qubernetic.copia-cli
    ```

=== "Fedora/RHEL"

    ```bash
    sudo dnf copr enable cbiro/copia-cli
    sudo dnf install copia-cli
    ```

Check the result with `copia-cli --version`. The [installation pages](../install/index.md) list every other method: `.deb`, Snap, AUR, release archives, and building from source.

## 2. Sign in

Create a personal access token in Copia under **Settings > Applications** ([the steps](authentication.md#create-a-token)), then run:

```bash
copia-cli auth login
```

It asks `Enter token for app.copia.io:`. Paste the token. When Copia accepts it, the command prints:

```text
Logged in as <your login> on app.copia.io
```

The token goes into your OS keyring, or into the config file when no keyring is available. For another Copia host, add `--host`; for CI, see [Authentication](authentication.md).

## 3. Clone a repository

```bash
copia-cli repo clone my-org/my-repo
```

git prints its usual clone output, and the repository lands in the directory `my-repo`. The clone authenticates with your token, which is not written into the remote URL or a credential store. Change into the directory: the next commands find the repository from its `origin` remote.

```bash
cd my-repo
```

## 4. List and create issues

```bash
copia-cli issue list
```

It prints the open issues, one per line: number, title, state and labels.

```text
#12  Conveyor stops after E-stop reset  open  bug
#9   Add alarm text for drive fault     open
```

File a new one:

```bash
copia-cli issue create --title "Fix sensor mapping"
```

It prints the number, the title and the issue's address:

```text
Created issue #13: Fix sensor mapping
https://app.copia.io/my-org/my-repo/issues/13
```

## 5. Open a pull request

Push a branch with your change, then open a pull request from it into `main` (use your repository's default branch):

```bash
copia-cli pr create --title "Fix sensor mapping" --base main --head fix/sensor-mapping
```

It prints the pull request's number, title and address:

```text
Created PR #14: Fix sensor mapping
https://app.copia.io/my-org/my-repo/pulls/14
```

## Next

- `copia-cli pr list`, `pr view`, `pr review` and `pr merge` take the pull request further.
- `copia-cli status` lists the issues and pull requests assigned to you, your review requests and mentions, and your latest notifications.
- The [command reference](../manual/copia-cli.md) describes every command and option.
