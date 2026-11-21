#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

VERSION="${VERSION:-0.2.2}"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo none)"
DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
PACKAGE="github.com/Bilalmesk/ffmpeg-batch-convert/internal/version"

mkdir -p dist
echo "Building dist/ffbatch ..."
go build -trimpath \
  -ldflags "-s -w -X ${PACKAGE}.Version=${VERSION} -X ${PACKAGE}.Commit=${COMMIT} -X ${PACKAGE}.Date=${DATE}" \
  -o dist/ffbatch .

ls -lh dist/ffbatch
./dist/ffbatch version
