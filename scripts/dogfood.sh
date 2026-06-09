#!/bin/bash
# dogfood.sh — Pipes AI Code Review (VOID) container logs into LogStream
# Usage: ./scripts/dogfood.sh

LOGSTREAM_URL="http://localhost:8090"
API_KEY="dev-key"
LOGSTREAM_DIR="$(cd "$(dirname "$0")/.." && pwd)"

echo "=== LogStream Dogfooding Pipeline ==="
echo "Source: VOID containers → LogStream @ $LOGSTREAM_URL"
echo ""

# Step 1: Build the binary ONCE (instead of 5x 'go run' fighting over build cache)
echo "Building logstream binary..."
BINARY="$LOGSTREAM_DIR/.logstream-collector"
go build -o "$BINARY" "$LOGSTREAM_DIR/cmd/logstream/main.go"
if [ $? -ne 0 ]; then
    echo "ERROR: Build failed!"
    exit 1
fi
echo "Build OK."
echo ""

# Trap Ctrl+C to kill all background collectors and clean up
trap 'echo ""; echo "Stopping all collectors..."; kill $(jobs -p) 2>/dev/null; exit 0' INT TERM

# Step 2: Start collectors — use --since=1m to grab recent context, then follow
# (--tail 0 misses logs generated during build time; --since=1m catches those)

echo "  → Tailing ai-code-review-webhook-handler-1..."
docker logs --tail 1 -f ai-code-review-webhook-handler-1 2>&1 | \
  "$BINARY" collect \
    --service=webhook-handler \
    --format=docker \
    --url="$LOGSTREAM_URL" \
    --api-key="$API_KEY" &

echo "  → Tailing ai-code-review-api-server-1..."
docker logs --tail 1 -f ai-code-review-api-server-1 2>&1 | \
  "$BINARY" collect \
    --service=api-server \
    --format=docker \
    --url="$LOGSTREAM_URL" \
    --api-key="$API_KEY" &

echo "  → Tailing ai-code-review-review-worker-1..."
docker logs --tail 1 -f ai-code-review-review-worker-1 2>&1 | \
  "$BINARY" collect \
    --service=review-worker \
    --format=docker \
    --url="$LOGSTREAM_URL" \
    --api-key="$API_KEY" &

echo "  → Tailing ai-code-review-code-intelligence-1..."
docker logs --tail 1 -f ai-code-review-code-intelligence-1 2>&1 | \
  "$BINARY" collect \
    --service=code-intelligence \
    --format=docker \
    --url="$LOGSTREAM_URL" \
    --api-key="$API_KEY" &

echo "  → Tailing ai-code-review-postgres-1..."
docker logs --tail 1 -f ai-code-review-postgres-1 2>&1 | \
  "$BINARY" collect \
    --service=postgres \
    --format=docker \
    --url="$LOGSTREAM_URL" \
    --api-key="$API_KEY" &

echo ""
echo "All 5 collectors running. Press Ctrl+C to stop."
echo "Open http://localhost:5173/tail to see logs streaming."
echo ""

wait