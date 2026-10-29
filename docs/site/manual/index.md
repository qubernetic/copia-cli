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

- Run `copia-cli auth login` to authenticate with your Copia instance. Alternatively, `copia-cli` will respect the `COPIA_TOKEN` [environment variable](https://github.com/qubernetic/copia-cli#environment-variables).
- To target a specific Copia host, use the `--host` flag or set `COPIA_HOST`.

## Examples

```bash
$ copia-cli issue list
$ copia-cli issue list --assignee @me --milestone v1.0
$ copia-cli issue create --label bug
$ copia-cli repo view my-org/my-repo
```

## See also

- [copia-cli](./copia-cli)
