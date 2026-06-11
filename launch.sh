#!/usr/bin/env bash
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
URL="http://127.0.0.1:18596"
PROFILE="trading-db"

# Start server in background
cd "$DIR"
go run . &
SERVER_PID=$!

# Make sure the server is stopped whenever this script exits.
# `go run` spawns the compiled binary as a child, so we kill that child too.
cleanup() {
    pkill -P "$SERVER_PID" 2>/dev/null
    kill "$SERVER_PID" 2>/dev/null
}
trap cleanup EXIT INT TERM

# Wait for Gin to be ready
echo "Waiting for server..."
until curl -sf "$URL" > /dev/null 2>&1; do
    sleep 0.5
done

# Launch Firefox in the foreground; --no-remote keeps it as our child process
# so this returns only once the window is closed
firefox -P "$PROFILE" --no-remote "$URL"

# Firefox has been closed -> stop the server (handled by the EXIT trap)
