#!/bin/bash
# Build the Rule Studio desktop application using Wails.
# Usage: ./build-desktop.sh [dev|build]
set -e

cd "$(dirname "$0")"
MODE="${1:-build}"

if ! command -v wails &> /dev/null; then
    echo "Error: wails CLI not found. Install with:"
    echo "  go install github.com/wailsapp/wails/v2/cmd/wails@latest"
    exit 1
fi

cd desktop

if [ "$MODE" = "dev" ]; then
    echo "🔬 Starting Rule Studio in development mode..."
    wails dev
elif [ "$MODE" = "build" ]; then
    echo "🔬 Building Rule Studio desktop app..."
    wails build
    echo "✅ Built: desktop/build/bin/rule-studio"
else
    echo "Usage: $0 [dev|build]"
    exit 1
fi
