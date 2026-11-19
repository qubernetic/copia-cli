# Installing copia-cli on Windows

## Recommended _(Official)_

### WinGet

```powershell
winget install Qubernetic.copia-cli
```

To upgrade:

```powershell
winget upgrade --id Qubernetic.copia-cli
```

!!! note
    An upgrade can move the `copia-cli` command to another directory on your `PATH`. If `copia-cli` is not found after the upgrade, **open a new terminal window**.

### Precompiled binaries

[Copia CLI releases](https://github.com/qubernetic/copia-cli/releases/latest) contain precompiled `.zip` archives for `amd64`, `arm64`, and `386` architectures.

1. Download the appropriate `.zip` from [GitHub Releases](https://github.com/qubernetic/copia-cli/releases/latest)
2. Extract `copia-cli.exe`
3. Add the directory containing `copia-cli.exe` to your `PATH`

!!! note
    When using Windows Terminal, you will need to **open a new window** for PATH changes to take effect.

## Building from source

See [Build from source](source.md).
