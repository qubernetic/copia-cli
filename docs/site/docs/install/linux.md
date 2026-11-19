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

Download the `.deb` package for your architecture from the [latest release](https://github.com/qubernetic/copia-cli/releases/latest). Its file name holds the version, for example `copia-cli_<version>_linux_amd64.deb`. Then install it, with your file's name in place of the example:

```bash
sudo dpkg -i copia-cli_<version>_linux_amd64.deb
```

### Fedora/RHEL (COPR)

```bash
sudo dnf copr enable @qubernetic/copia-cli
sudo dnf install copia-cli
```

To upgrade:

```bash
sudo dnf upgrade copia-cli
```

The COPR project moved from `cbiro/copia-cli` to `@qubernetic/copia-cli`. If you enabled the old one, switch to the new one before you upgrade:

```bash
sudo dnf copr remove cbiro/copia-cli
sudo dnf copr enable @qubernetic/copia-cli
```

Alternatively, download the `.rpm` package for your architecture from the [latest release](https://github.com/qubernetic/copia-cli/releases/latest) and install it, with your file's name in place of the example:

```bash
sudo dnf install -y copia-cli_<version>_linux_amd64.rpm
```

### Snap

```bash
sudo snap install copia-cli
```

!!! note
    The Snap is built for `amd64` only. On other architectures, use a release binary or another installation method.

!!! note
    The Snap uses strict confinement: it can only access files in your home directory. If you work with git repositories outside `$HOME`, use a different installation method.

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

See [Build from source](source.md).
