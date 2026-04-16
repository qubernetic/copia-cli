<div align="center">

# Copia CLI

[![CI](https://github.com/qubernetic/copia-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/qubernetic/copia-cli/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8.svg?logo=go&logoColor=white)](https://go.dev/)
[![Release](https://img.shields.io/github/v/release/qubernetic/copia-cli?include_prereleases)](https://github.com/qubernetic/copia-cli/releases)
[![License: AGPL-3.0](https://img.shields.io/badge/license-AGPL--3.0-blue.svg)](LICENSE)
[![codecov](https://codecov.io/gh/qubernetic/copia-cli/graph/badge.svg?token=53E14B9UE1)](https://codecov.io/gh/qubernetic/copia-cli)

</div>

`copia-cli` is [Copia](https://copia.io) on the command line. It brings repositories, issues, pull requests, and other Copia concepts to the terminal next to where you are already working with `git` and your code.

Copia CLI is supported for users on [Copia Cloud](https://app.copia.io), with support for Linux, macOS, and Windows.

## Documentation

For [installation options see below](#installation), for usage instructions [see the manual](https://qubernetic.github.io/copia-cli/manual/).

## Contributing

If anything feels off, or if you feel that some functionality is missing, please check out the [contributing page](CONTRIBUTING.md). There you will find instructions for sharing your feedback, building the tool locally, and submitting pull requests to the project.

<!-- this anchor is linked to from elsewhere, so avoid renaming it -->
## Installation

### [macOS](docs/install_macos.md)

- [Homebrew](docs/install_macos.md#homebrew)
- [Precompiled binaries](docs/install_macos.md#precompiled-binaries) on [releases page][]

### [Linux](docs/install_linux.md)

- [Homebrew](docs/install_linux.md#homebrew)
- [Debian/Ubuntu (.deb)](docs/install_linux.md#debianubuntu-deb)
- [Fedora/RHEL (.rpm)](docs/install_linux.md#fedorarhel-rpm)
- [Precompiled binaries](docs/install_linux.md#precompiled-binaries) on [releases page][]

### [Windows](docs/install_windows.md)

- [WinGet](docs/install_windows.md#winget)
- [Precompiled binaries](docs/install_windows.md#precompiled-binaries) on [releases page][]

### Build from source

See here on how to [build Copia CLI from source](docs/install_source.md).

## Quick Start

```bash
# Authenticate
$ copia-cli auth login --host app.copia.io --token YOUR_TOKEN

# List your repos
$ copia-cli repo list

# Create an issue
$ copia-cli issue create --title "Fix sensor mapping" --label bug

# Open a PR
$ copia-cli pr create --title "feat: add safety interlock" --base develop

# Merge it
$ copia-cli pr merge 7 --merge --delete-branch
```

## Shell Completion

Shell completion is available for Bash, Zsh, Fish, and PowerShell. See `copia-cli completion --help` or the [manual](https://qubernetic.github.io/copia-cli/manual/copia-cli_completion) for setup instructions.

## Roadmap

- **Phase 1 (MVP):** auth, repo list/view/clone, issue CRUD, pr CRUD, label list/create — **Done**
- **Phase 2 (Workflow):** release CRUD, repo create/delete/fork, pr review/diff/checkout, issue edit, Homebrew tap — **Done**
- **Phase 3 (Power Features):** `api` escape hatch, search, orgs, notifications, `-R`/`--repo` flag, completion, Jekyll manual, AGPL license — **Done**
- **Phase 4 (Nice to Have):** winget, OS keyring, aliases, browse, status dashboard, ssh-key, pr checks, scheduled integration tests

## License

[AGPL-3.0](LICENSE) — see [LICENSE-COMMERCIAL.md](LICENSE-COMMERCIAL.md) for commercial licensing options.

[releases page]: https://github.com/qubernetic/copia-cli/releases/latest
