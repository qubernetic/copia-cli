# copia-cli

> Work with repositories, issues, pull requests and releases on [Copia](https://copia.io), the source control platform for industrial automation, from the terminal where you already run `git`.

[Install](install/index.md){ .md-button .md-button--primary }
[Get started](getting-started/quick-start.md){ .md-button }
[Command reference](manual/copia-cli.md){ .md-button }

## What it does

copia-cli calls Copia's API with your personal access token. You can list, clone and create repositories, file and edit issues, open, review and merge pull requests, publish releases and read notifications, so reviewing a change to a PLC project no longer means switching to the web UI. `copia-cli status` lists the issues and pull requests waiting for you, and `copia-cli api` reaches any endpoint the other commands do not cover.

```bash
copia-cli auth login
copia-cli repo clone my-org/my-repo
cd my-repo
copia-cli issue list
copia-cli pr create --title "Fix sensor mapping" --base main --head fix/sensor-mapping
```

The [quick start](getting-started/quick-start.md) walks through these commands one by one.
