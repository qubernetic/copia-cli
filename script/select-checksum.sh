#!/bin/sh
# Prints the checksum of asset $2 from the checksums file $1 (the goreleaser
# shape, "<sha256>  <name>" per line) and fails unless exactly one line names
# the asset exactly. aur.yml runs it for each archive it puts into PKGBUILD,
# before script/check-sha256.sh checks the value.
set -eu

file="${1-}"
asset="${2-}"

# The name is compared as a string, never as a pattern; ENVIRON keeps awk
# from reading backslash escapes in it the way -v would.
if ! ASSET="$asset" awk '
  NF == 2 && $2 == ENVIRON["ASSET"] { sum = $1; n++ }
  END { if (n != 1) exit 1; print sum }
' "$file"; then
  printf '::error::expected exactly one checksum line for "%s"\n' "$asset" >&2
  exit 1
fi
