---
layout: manual
permalink: /manual/
title: Copia CLI Manual
---

# Copia CLI manual

`copia-cli`, or `copia`, is a command-line interface for Copia for use in your terminal or your scripts.

- [Available commands](./copia-cli)

## Installation

You can find installation instructions on our [README](https://github.com/qubernetic/copia-cli#installation).

## Configuration

- Run `copia-cli auth login` to authenticate with your Copia instance. Alternatively, `copia-cli` will respect the `COPIA_TOKEN` [environment variable](https://github.com/qubernetic/copia-cli/blob/main/docs/authentication.md#auth-precedence) together with `COPIA_HOST` (or `--host`): it is sent only to the host they name, and without one of them it is ignored with a warning.
- To target a specific Copia host, use the `--host` flag or set `COPIA_HOST`.
- Run `copia-cli config list` to see the settings (`output`, `browser`, `pager`, `editor`) and `copia-cli config set <key> <value>` to change one.
- Run `copia-cli alias set <name> <expansion>` to add a shortcut for a command, for example `copia-cli alias set co 'pr checkout'`.

## Examples

```bash
$ copia-cli issue list
$ copia-cli issue list --assignee @me --milestone v1.0
$ copia-cli issue create --label bug
$ copia-cli repo view my-org/my-repo
```

## See also

- [copia-cli](./copia-cli)
