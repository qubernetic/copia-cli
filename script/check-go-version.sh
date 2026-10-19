#!/bin/sh
# Fails unless $1 is a Go release version such as 1.27 or 1.26.9: two or
# three components, each 0 or up to four digits without a leading zero.
# bump-go.yml runs it on the version read from go.dev and on the one read
# from go.mod before either reaches GITHUB_OUTPUT, where they later name a
# branch and a PR.
set -eu

version="${1-}"

. "$(dirname "$0")/lib/workflow-escape.sh"

# The closing quote keeps a trailing newline in the value visible to
# workflow_escape.
reject() {
  printf '::error::not a Go version: "%s"\n' "$version" | workflow_escape >&2
  exit 1
}

# grep matches line by line, so a value with a newline is rejected first.
case "$version" in
  *'
'*) reject ;;
esac

n='(0|[1-9][0-9]{0,3})'
printf '%s\n' "$version" | grep -Eqx "$n\.$n(\.$n)?" || reject
