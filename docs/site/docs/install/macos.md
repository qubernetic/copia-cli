# Installing copia-cli on macOS

## Recommended _(Official)_

### Homebrew

```bash
brew install qubernetic/tap/copia-cli
```

To upgrade:

```bash
brew upgrade qubernetic/tap/copia-cli
```

### Precompiled binaries

[Copia CLI releases](https://github.com/qubernetic/copia-cli/releases/latest) contain a universal macOS binary (amd64+arm64).

```bash
# Download and extract
tar xzf copia-cli_*_darwin_all.tar.gz

# Move to PATH
sudo mv copia-cli /usr/local/bin/
```

## Building from source

See [Build from source](source.md).
