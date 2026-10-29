# Dev image for the sandbox (.devcontainer/compose.yaml). The source is bind-mounted, not copied,
# and the image runs as 1000:100, so nothing here writes git config as root.
FROM golang:1.26.9-alpine

ARG GOLANGCI_LINT_VERSION=v2.15.0

RUN apk add --no-cache git git-daemon build-base

# Built with the image's Go, so the linter understands the go.mod Go version. The module proxy
# checksum database verifies the pinned version.
RUN GOBIN=/usr/local/bin go install "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_LINT_VERSION}" \
    && rm -rf /root/.cache /go/pkg/mod

# The named volumes in compose.yaml mount here; Docker copies this ownership into a fresh volume.
ENV HOME=/home/dev \
    GOCACHE=/home/dev/.cache/go-build \
    GOMODCACHE=/home/dev/go/pkg/mod
RUN mkdir -p "$GOCACHE" "$GOMODCACHE" && chown -R 1000:100 "$HOME"

USER 1000:100
WORKDIR /workspace
