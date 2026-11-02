#!/usr/bin/env bash
# Build small clients. Each client downloads its own platform's tools on demand.
set -euo pipefail
cd "$(dirname "$0")/.."
VERSION="${VERSION:-0.1.0}"
mkdir -p dist
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do
  os=${target%/*}; arch=${target#*/}
  suffix=""
  if [ "$os" = windows ]; then suffix=".exe"; fi
  GOOS=$os GOARCH=$arch CGO_ENABLED=0 go build -buildvcs=false \
    -ldflags "-s -w -X laraserve/internal/runtime.Version=$VERSION" \
    -o "dist/laraserve-$os-$arch$suffix" .
  echo "built dist/laraserve-$os-$arch$suffix"
done
