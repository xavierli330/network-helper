#!/bin/bash
# build.sh - 构建脚本，支持版本号注入

set -e

VERSION=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}
LDFLAGS="-X main.version=${VERSION}"

echo "Building nethelper ${VERSION}..."
go build -ldflags "${LDFLAGS}" -o nethelper ./cmd/nethelper

echo "Build complete: ./nethelper"
./nethelper version
