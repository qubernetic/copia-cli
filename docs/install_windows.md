# Installing copia-cli on Windows

## Recommended _(Official)_

### WinGet

```pwsh
winget install Qubernetic.copia-cli
```

### Precompiled binaries

[Copia CLI releases](https://github.com/qubernetic/copia-cli/releases/latest) contain precompiled `.zip` archives for `amd64`, `arm64`, and `386` architectures.

1. Download the appropriate `.zip` from [GitHub Releases](https://github.com/qubernetic/copia-cli/releases/latest)
2. Extract `copia-cli.exe`
3. Add the directory containing `copia-cli.exe` to your `PATH`

> [!NOTE]
> When using Windows Terminal, you will need to **open a new window** for PATH changes to take effect.

## Community _(Unofficial)_

### Scoop

```pwsh
scoop install copia-cli
```

> [!NOTE]
> Scoop support is community-maintained and may lag behind official releases.

## Building from source

See [install_source.md](install_source.md).
