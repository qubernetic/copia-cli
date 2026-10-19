#!/bin/sh
# Writes version=<X.Y.Z> to GITHUB_OUTPUT, taken from the release tag in
# GITHUB_REF_NAME without its leading v, and writes nothing unless the value
# passes script/check-release-version.sh. The Extract version steps of
# aur.yml and copr.yml are this one line, so the workflow test can pin the
# gate by the step's run block alone.
set -eu

version="${GITHUB_REF_NAME#v}"
"$(dirname "$0")/check-release-version.sh" "$version"
printf 'version=%s\n' "$version" >> "$GITHUB_OUTPUT"
