#!/bin/bash
# cleanup.sh - Clean up all Orion services and reset etcd state

set -e

PID_FILE="/tmp/orion_manager.pid"
MANAGER_BIN="/tmp/orion_manager_bin"

echo "🧹 Cleaning up Orion system..."

# Kill all go processes
echo "→ Killing manager and worker processes..."
if [ -f "$PID_FILE" ]; then
	pid="$(cat "$PID_FILE")"
	if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
		kill "$pid" 2>/dev/null || true
		sleep 1
		if kill -0 "$pid" 2>/dev/null; then
			kill -9 "$pid" 2>/dev/null || true
		fi
	fi
	rm -f "$PID_FILE"
fi

# Fallback cleanup for any old go-run spawned manager binaries
pkill -9 -f "/tmp/go-build.*/exe/main|/\.cache/go-build/.*/main" || true
rm -f "$MANAGER_BIN"
sleep 1

# Stop docker containers
echo "→ Stopping Docker containers..."
cd "$(dirname "$0")/.."
docker compose down || true

# Wait for Docker to fully shut down
sleep 2

# Clean up docker volumes (optional - remove if you want to keep etcd data)
echo "→ Removing etcd volumes..."
docker volume rm orion_etcd_data 2>/dev/null || true

echo "✓ Cleanup complete!"
echo ""
echo "Next step: Run './scripts/start.sh' to restart the system"
