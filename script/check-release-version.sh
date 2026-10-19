#!/bin/sh
# Fails unless $1 is a release version of the form X.Y.Z, such as 1.2.3.
# A pre-release such as 1.2.3-rc.1 is rejected: makepkg refuses a pkgver
# and rpmbuild a Version: that contains a hyphen.
# aur.yml and copr.yml run it on the version taken from the release tag
# before the value reaches GITHUB_OUTPUT, where it later becomes part of
# PKGBUILD, .SRCINFO, a sed program and the RPM spec.
set -eu

version="${1-}"

reject() {
  printf '::error::not a release version: "%s"\n' "$version" >&2
  exit 1
}

# grep matches line by line, so a value with a newline is rejected first.
case "$version" in
  *'
'*) reject ;;
esac

printf '%s\n' "$version" | grep -Eqx '[0-9]+\.[0-9]+\.[0-9]+' || reject
