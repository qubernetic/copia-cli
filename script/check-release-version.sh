#!/bin/sh
# Fails unless $1 is a release version such as 1.2.3 or 1.2.3-rc.1.
# aur.yml and copr.yml run it on the version taken from the release tag
# before the value reaches GITHUB_OUTPUT, where it later becomes part of
# PKGBUILD, .SRCINFO, a sed program and the RPM spec.
set -eu

version="${1-}"

. "$(dirname "$0")/lib/workflow-escape.sh"

# The closing quote keeps a trailing newline in the value visible to
# workflow_escape.
reject() {
  printf '::error::not a release version: "%s"\n' "$version" | workflow_escape >&2
  exit 1
}

# grep matches line by line, so a value with a newline is rejected first.
case "$version" in
  *'
'*) reject ;;
esac

printf '%s\n' "$version" | grep -Eqx '[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?' || reject
