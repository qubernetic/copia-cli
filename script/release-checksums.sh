#!/bin/sh
# Writes sha256_x86_64= and sha256_aarch64= to GITHUB_OUTPUT: the checksums
# of the linux amd64 and arm64 archives of release version $2, read from the
# checksums file $1 with script/select-checksum.sh. Writes nothing unless both
# pass script/check-sha256.sh. The Get checksums step of aur.yml is this one
# line, so the workflow test can pin the gate by the step's run block alone.
set -eu

file="${1-}"
version="${2-}"
dir="$(dirname "$0")"

sha_x86="$("$dir/select-checksum.sh" "$file" "copia-cli_${version}_linux_amd64.tar.gz")"
sha_arm="$("$dir/select-checksum.sh" "$file" "copia-cli_${version}_linux_arm64.tar.gz")"
"$dir/check-sha256.sh" "$sha_x86"
"$dir/check-sha256.sh" "$sha_arm"
printf 'sha256_x86_64=%s\nsha256_aarch64=%s\n' "$sha_x86" "$sha_arm" >> "$GITHUB_OUTPUT"
