#!/bin/sh
# Fails unless $1 is a Go release version such as 1.27 or 1.26.9: two or
# three components, each 0 or up to four digits without a leading zero.
# bump-go.yml runs it on the version read from go.dev before the value
# reaches GITHUB_OUTPUT, where it later names a branch and a PR.
set -eu

version="${1-}"

# The runner reads every log line as a possible workflow command, so the
# value is printed with the workflow-command encoding: % as %25, CR as %0D
# and LF as %0A. The closing quote keeps a trailing newline visible to sed.
reject() {
  cr=$(printf '\r')
  printf '::error::not a Go version: "%s"\n' "$version" |
    sed -e 's/%/%25/g' -e "s/$cr/%0D/g" |
    sed -e ':a' -e '$!N' -e 's/\n/%0A/' -e 'ta' >&2
  exit 1
}

# grep matches line by line, so a value with a newline is rejected first.
case "$version" in
  *'
'*) reject ;;
esac

n='(0|[1-9][0-9]{0,3})'
printf '%s\n' "$version" | grep -Eqx "$n\.$n(\.$n)?" || reject
