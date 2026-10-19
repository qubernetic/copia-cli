#!/bin/sh
# Fails unless $1 is a lowercase hex SHA-256 checksum.
# aur.yml runs it on each checksum read from the release's checksums.txt
# before the value reaches GITHUB_OUTPUT, where it later becomes part of
# PKGBUILD and .SRCINFO.
set -eu

sum="${1-}"

. "$(dirname "$0")/lib/workflow-escape.sh"

# The closing quote keeps a trailing newline in the value visible to
# workflow_escape.
reject() {
  printf '::error::not a sha256 checksum: "%s"\n' "$sum" | workflow_escape >&2
  exit 1
}

# grep matches line by line, so a value with a newline is rejected first.
case "$sum" in
  *'
'*) reject ;;
esac

printf '%s\n' "$sum" | grep -Eqx '[0-9a-f]{64}' || reject
