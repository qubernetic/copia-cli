#!/bin/sh
# Fails unless $1 is a lowercase hex SHA-256 checksum.
# aur.yml runs it on each checksum read from the release's checksums.txt
# before the value reaches GITHUB_OUTPUT, where it later becomes part of
# PKGBUILD and .SRCINFO.
set -eu

sum="${1-}"

# The runner reads every log line as a possible workflow command, so the
# value is printed with the workflow-command encoding: % as %25, CR as %0D
# and LF as %0A. The closing quote keeps a trailing newline visible to sed.
reject() {
  cr=$(printf '\r')
  printf '::error::not a sha256 checksum: "%s"\n' "$sum" |
    sed -e 's/%/%25/g' -e "s/$cr/%0D/g" |
    sed -e ':a' -e '$!N' -e 's/\n/%0A/' -e 'ta' >&2
  exit 1
}

# grep matches line by line, so a value with a newline is rejected first.
case "$sum" in
  *'
'*) reject ;;
esac

printf '%s\n' "$sum" | grep -Eqx '[0-9a-f]{64}' || reject
