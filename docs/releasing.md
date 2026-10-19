# Releasing

## Process

1. Create a release branch from `develop`:

   ```bash
   $ git checkout -b release/vX.Y.Z develop
   ```

2. Update `CHANGELOG.md` with the release version and date

3. Commit and push:

   ```bash
   $ git commit -m "chore(release): prepare vX.Y.Z"
   $ git push -u origin release/vX.Y.Z
   ```

4. Create a PR to `main` and merge

5. Tag the release on `main`:

   ```bash
   $ git checkout main && git pull
   $ git tag vX.Y.Z
   $ git push origin vX.Y.Z
   ```

   Only a repository admin can create a `v*` tag (ruleset `Release tag creation`, Repository admin bypass). Once pushed, a `v*` tag cannot be deleted or moved by anyone, admins included (ruleset `Protected release tags`). Check the commit before you push: a wrong tag means a new patch version.

6. The tag push triggers the [Release workflow](.github/workflows/release.yml) which:

   - Builds binaries for Linux, macOS, and Windows
   - Creates `.deb` and `.rpm` packages
   - Creates a macOS universal binary
   - Uploads all artifacts to a GitHub Release (draft)
   - Updates the [Homebrew formula](https://github.com/qubernetic/homebrew-tap)

7. Review and publish the draft release on GitHub. Publishing triggers:

   - [WinGet](.github/workflows/winget.yml) — updates `Qubernetic.copia-cli` in winget-pkgs
   - [COPR](.github/workflows/copr.yml) — builds and publishes RPM to Fedora COPR
   - [AUR](.github/workflows/aur.yml) — updates `copia-cli-bin` on AUR
   - [Snap](.github/workflows/snap.yml) — builds and publishes to the Snap Store

   COPR and AUR stop before they publish anything unless the tag is `vX.Y.Z` (digits only, for example `v1.2.3`), and AUR also stops when `checksums.txt` does not have exactly one line for each Linux archive (`copia-cli_X.Y.Z_linux_amd64.tar.gz`, `copia-cli_X.Y.Z_linux_arm64.tar.gz`) or a checksum there is not a SHA-256 hex string.

8. Back-merge `main` into `develop`:

   ```bash
   $ git checkout develop
   $ git merge main
   $ git push origin develop
   ```

## GoReleaser

The release build is powered by [GoReleaser](https://goreleaser.com/). Configuration is in `.goreleaser.yml`.

To test the release locally without publishing:

```bash
$ just snapshot
```

## Pre-releases

Pre-releases use the `vX.Y.Z-rc.N` or `vX.Y.Z-beta.N` tag format. GoReleaser detects the pre-release tag and marks the GitHub Release accordingly.

Pre-releases are not published to AUR or COPR: both workflows run only on a published full release, and `script/check-release-version.sh` rejects a pre-release version because makepkg and rpmbuild do not accept a hyphen in the package version.

Version progression: `beta.1` → `rc.1` → `rc.2` → stable.
