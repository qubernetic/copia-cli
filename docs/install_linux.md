# Installing copia-cli on Linux

## Recommended _(Official)_

### Homebrew

```bash
brew install qubernetic/tap/copia-cli
```

To upgrade:

```bash
brew upgrade qubernetic/tap/copia-cli
```

### Debian/Ubuntu (.deb)

Download the `.deb` package from [GitHub Releases](https://github.com/qubernetic/copia-cli/releases/latest):

```bash
# Download the latest .deb (amd64)
curl -LO https://github.com/qubernetic/copia-cli/releases/latest/download/copia-cli_*_linux_amd64.deb

# Install
sudo dpkg -i copia-cli_*_linux_amd64.deb
```

### Fedora/RHEL (COPR)

```bash
sudo dnf copr enable qubernetic/copia-cli
sudo dnf install copia-cli
```

To upgrade:

```bash
sudo dnf upgrade copia-cli
```

Alternatively, download the `.rpm` package directly from [GitHub Releases](https://github.com/qubernetic/copia-cli/releases/latest):

```bash
curl -LO https://github.com/qubernetic/copia-cli/releases/latest/download/copia-cli_*_linux_amd64.rpm
sudo dnf install -y copia-cli_*_linux_amd64.rpm
```

### Snap

```bash
sudo snap install copia-cli --classic
```

### AUR (Arch Linux)

```bash
yay -S copia-cli-bin
```

Or with any other AUR helper. The package is [`copia-cli-bin`](https://aur.archlinux.org/packages/copia-cli-bin).

### Precompiled binaries

[Copia CLI releases](https://github.com/qubernetic/copia-cli/releases/latest) contain precompiled binaries for `amd64`, `arm64`, `386`, and `armv6` architectures.

```bash
# Download and extract
tar xzf copia-cli_*_linux_amd64.tar.gz

# Move to PATH
sudo mv copia-cli /usr/local/bin/
```

## Building from source

See [install_source.md](install_source.md).
