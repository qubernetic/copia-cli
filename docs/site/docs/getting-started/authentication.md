# Authentication

> Every Copia API call needs a token. Sign in once with `auth login`, and the other commands use the stored token.

## Create a token

1. Log in to the Copia web UI.
2. Go to **Settings > Applications**.
3. Under "Manage Access Tokens", enter a name, for example `copia-cli`.
4. Click **Generate Token**.
5. Copy the token right away. Copia shows it only once.

## Sign in

```bash
copia-cli auth login
```

Without `--token`, and with standard input on a terminal, the command asks for the token (`Enter token for app.copia.io: `). The host defaults to `app.copia.io`; use `--host` for another Copia instance. In a script or CI job, pass both flags so nothing is asked:

```bash
copia-cli auth login --host copia.example.com --token <your-token>
```

`auth login` checks the token against the host, then prints `Logged in as <login> on <host>`. The host becomes the default host, which the other commands use when you name no host. See [`auth login`](../manual/copia-cli_auth_login.md) for every option.

## Where the token is kept

`auth login` stores the token in the OS keyring: the macOS Keychain, the Windows Credential Manager, or the Secret Service on Linux (GNOME Keyring, KWallet). The config file then holds the host and the account, but no token. It is `~/.config/copia/config.yml` on Linux, macOS and Windows (on Windows, `%USERPROFILE%\.config\copia\config.yml`). When `XDG_CONFIG_HOME` is set, it is `$XDG_CONFIG_HOME/copia/config.yml` instead.

Sometimes there is no keyring to use: a headless server, a container or a CI runner without a Secret Service, or a locked keyring that does not answer within three seconds. Then `auth login` stores the token in plain text in the config file and prints a warning on standard error that names the file. The warning never contains the token. To store the token in the config file on purpose, without trying the keyring, add `--insecure-storage`:

```bash
copia-cli auth login --token <your-token> --insecure-storage
```

The Snap package cannot reach the keyring, so it always stores the token in the config file and prints the warning.

The config file is written with permissions `600`, readable by your user only.

`auth logout` removes the account and deletes its token from the keyring and from the config file. It does not revoke the token on Copia.

## Use an environment variable

For scripts and CI, give the token in `COPIA_TOKEN` and the host it belongs to in `COPIA_HOST`:

```bash
export COPIA_HOST=app.copia.io
export COPIA_TOKEN="$COPIA_CI_TOKEN"
copia-cli repo list
```

`COPIA_TOKEN` is used only when the host comes from `COPIA_HOST`, or from `--host` with `COPIA_HOST` unset or naming the same host. When the host comes from the config file instead, the commands ignore `COPIA_TOKEN`, use the stored token and print a warning on standard error. That way the token never goes to a host you did not name.

## Several accounts on one host

Log in with the token of another account on the same host and copia-cli adds that account and makes it the host's active account. The accounts you logged in with before stay stored, each with its own token. The commands always use the active account's token.

`auth switch` makes another stored account active and its host the default host, then prints `Switched active account for <host> to <login>`:

```bash
copia-cli auth switch
copia-cli auth switch --host app.copia.io --user john
```

`auth switch` picks from the stored accounts that match `--host` and `--user`, or from all of them without flags. With one match it switches to that account; with two, to the one not in use. When it cannot decide, it fails and lists the matching accounts, one `<host> <login>` per line, so you can name one with `--host` and `--user`. It does not prompt.

`auth logout --user <login>` removes one account. When the active account goes, the alphabetically first remaining account of that host becomes active.

## Check what is stored

```bash
copia-cli auth status
```

`auth status` lists every host in the config file, the default host marked `(default)`, and under each host every account, the active one first. For each account it shows the user, whether it is active, the masked token with where it is stored (`keyring` or `config file`), and whether Copia accepts the token.

`auth status` reports on the stored tokens only. It does not read `COPIA_HOST` or `COPIA_TOKEN`, so with either set, the other commands may use a different host or token than the one it shows.

`--show-token` prints the token itself. Do not use it where the terminal is recorded or shared, or in a CI log.

## Use the token with git

`auth setup-git` makes copia-cli git's credential helper for the hosts you are logged in to, so `git clone`, `git fetch` and `git push` over https use the active account's token:

```bash
copia-cli auth setup-git                       # all hosts
copia-cli auth setup-git --host app.copia.io   # one host
```

It writes two entries per host to your global git config. Running it again changes nothing. `repo clone` does not need it: it gives git the token for the clone itself.

## Auth precedence

The host comes from `--host`, then `COPIA_HOST`, then the config's default host.

The token comes from, in this order:

1. the `--token` flag, for one command;
2. the `COPIA_TOKEN` environment variable, only when the host came from `COPIA_HOST`, or from `--host` with `COPIA_HOST` unset or naming the same host;
3. the stored token of the host's active account: the config file first, then the OS keyring.

The developer notes in [`docs/authentication.md`](https://github.com/qubernetic/copia-cli/blob/main/docs/authentication.md) cover the config file format, the warnings word for word, and calling the API directly.
