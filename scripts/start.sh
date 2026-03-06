#!/bin/bash
# start.sh - Start Orion orchestrator with proper initialization order

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(dirname "$SCRIPT_DIR")"
PID_FILE="/tmp/orion_manager.pid"
LOG_FILE="/tmp/orion_manager.log"
MANAGER_BIN="/tmp/orion_manager_bin"

echo "🚀 Starting Orion Orchestrator..."

stop_existing_manager() {
    if [ -f "$PID_FILE" ]; then
        old_pid="$(cat "$PID_FILE")"
        if [ -n "$old_pid" ] && kill -0 "$old_pid" 2>/dev/null; then
            echo "→ Stopping existing manager process (PID: $old_pid)..."
            kill "$old_pid" 2>/dev/null || true
            sleep 1
            if kill -0 "$old_pid" 2>/dev/null; then
                kill -9 "$old_pid" 2>/dev/null || true
            fi
        fi
        rm -f "$PID_FILE"
    fi

    # Fallback cleanup for older go-run based processes from previous runs
    pkill -9 -f "/tmp/go-build.*/exe/main|/\.cache/go-build/.*/main" >/dev/null 2>&1 || true
}

stop_existing_manager

# Step 1: Start etcd
echo ""
echo "→ Step 1: Starting etcd container..."
cd "$REPO_DIR"
docker compose up -d etcd

# Wait for etcd to be ready
echo "  Waiting for etcd to be healthy..."
for i in {1..30}; do
    if etcdctl --endpoints=localhost:12379 endpoint health > /dev/null 2>&1; then
        echo "  ✓ etcd is healthy"
        break
    fi
    echo "  ⏳ etcd not ready yet ($i/30)..."
    sleep 1
done

# Step 2: Clear stale state from etcd
echo ""
echo "→ Step 2: Clearing stale state from previous runs..."
etcdctl --endpoints=localhost:12379 del --prefix "/" || true
echo "  ✓ etcd state cleared"

# Step 3: Build binaries
echo ""
echo "→ Step 3: Building binaries..."
cd "$REPO_DIR"
go build -o orionctl ./cmd/orionctl
cd "$REPO_DIR/cmd/pkg"
go build -o "$MANAGER_BIN" .
echo "  ✓ CLI and manager binaries built"

# Step 4: Start the manager (in background)
echo ""
echo "→ Step 4: Starting manager..."
nohup "$MANAGER_BIN" > "$LOG_FILE" 2>&1 &
MANAGER_PID=$!
echo "$MANAGER_PID" > "$PID_FILE"
echo "  Manager PID: $MANAGER_PID"

echo "  Waiting for manager API readiness on port 8080..."
for i in {1..20}; do
    if curl -fsS http://127.0.0.1:8080/status >/dev/null 2>&1; then
        echo "  ✓ Manager API is ready"
        break
    fi
    if ! kill -0 "$MANAGER_PID" 2>/dev/null; then
        echo "  ✗ Manager exited unexpectedly"
        tail -n 50 "$LOG_FILE" || true
        exit 1
    fi
    echo "  ⏳ API not ready yet ($i/20)..."
    sleep 1
done

echo "  Resolving elected leader across manager ports..."
LEADER_PORT=""
for i in {1..20}; do
    for port in 8080 8081 8082; do
        status_json="$(curl -fsS "http://127.0.0.1:${port}/status" 2>/dev/null || true)"
        if echo "$status_json" | grep -q '"role":"leader"'; then
            LEADER_PORT="$port"
            break
        fi
    done
    if [ -n "$LEADER_PORT" ]; then
        break
    fi
    echo "  ⏳ Leader not visible yet ($i/20)..."
    sleep 1
done

if [ -n "$LEADER_PORT" ]; then
    echo "  ✓ Leader elected on http://localhost:${LEADER_PORT}"
else
    echo "  ⚠️  Leader not detected yet; API is up, retry CLI after a few seconds if needed"
fi

echo ""
echo "✅ Orion Orchestrator started successfully!"
echo ""
echo "📋 Available commands:"
echo "   ./orionctl get tasks --manager-addr http://localhost:8080"
echo "   ./orionctl get nodes --manager-addr http://localhost:8080"
echo "   ./orionctl run -f task.json --manager-addr http://localhost:8080"
echo "   ./orionctl stop <task-id> --manager-addr http://localhost:8080"
echo ""
echo "📝 Manager logs: tail -f $LOG_FILE"
echo "🧾 Manager PID file: $PID_FILE"
echo "🧹 To clean up: $SCRIPT_DIR/cleanup.sh"
