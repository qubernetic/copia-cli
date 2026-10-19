#!/bin/sh
# Fails unless $1 is a Go release version such as 1.27 or 1.26.9.
# bump-go.yml runs it on the version read from go.dev before the value
# reaches GITHUB_OUTPUT, where it later names a branch and a PR.
set -eu

version="${1-}"

reject() {
  printf '::error::not a Go version: "%s"\n' "$version" >&2
  exit 1
}

# grep matches line by line, so a value with a newline is rejected first.
case "$version" in
  *'
'*) reject ;;
esac

printf '%s\n' "$version" | grep -Eqx '[0-9]+\.[0-9]+(\.[0-9]+)?' || reject
