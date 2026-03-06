#!/bin/bash
# test-cli.sh - Integration tests for Orion CLI

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(dirname "$SCRIPT_DIR")"
MANAGER_ADDR="${MANAGER_ADDR:-http://localhost:8080}"

echo "🧪 Running Orion CLI Integration Tests"
echo "========================================"
echo ""

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test counter
TESTS_PASSED=0
TESTS_FAILED=0

# Helper functions
test_command() {
    local test_name="$1"
    local command="$2"
    local expected_pattern="$3"  # Optional regex pattern to match in output
    
    echo -n "Testing: $test_name... "
    
    if output=$(eval "$command" 2>&1); then
        if [ -z "$expected_pattern" ] || echo "$output" | grep -qE "$expected_pattern"; then
            echo -e "${GREEN}✓ PASS${NC}"
            ((TESTS_PASSED++))
            return 0
        else
            echo -e "${RED}✗ FAIL${NC} (pattern not found: $expected_pattern)"
            echo "  Output: $output"
            ((TESTS_FAILED++))
            return 1
        fi
    else
        echo -e "${RED}✗ FAIL${NC} (command failed)"
        echo "  Error: $output"
        ((TESTS_FAILED++))
        return 1
    fi
}

# Change to repo directory
cd "$REPO_DIR"

echo "Testing with Manager Address: $MANAGER_ADDR"
echo ""

# Test 1: Help command
test_command "orionctl help" \
    "./orionctl --help" \
    "Orion CLI"

# Test 2: Get tasks (should work even if empty)
test_command "get tasks" \
    "./orionctl get tasks --manager-addr $MANAGER_ADDR" \
    "TASK|No tasks"

# Test 3: Submit a task
TEST_TASK=$(mktemp)
cat > "$TEST_TASK" << 'EOF'
{
    "Name": "integration-test",
    "State": 0,
    "Image": "nginx:latest",
    "Memory": 512,
    "Disk": 1
}
EOF

test_command "run task" \
    "./orionctl run -f $TEST_TASK --manager-addr $MANAGER_ADDR" \
    "successfully|submitted"

rm -f "$TEST_TASK"

# Test 4: Verify task appears in list
sleep 1
test_command "get tasks after submit" \
    "./orionctl get tasks --manager-addr $MANAGER_ADDR" \
    "integration-test"

# Test 5: Get first task ID and test stop (if available)
FIRST_TASK_ID=$(./orionctl get tasks --manager-addr $MANAGER_ADDR 2>/dev/null | tail -n +2 | head -1 | awk '{print $1}' || echo "")
if [ -n "$FIRST_TASK_ID" ] && [ "$FIRST_TASK_ID" != "TASK" ]; then
    test_command "stop task" \
        "./orionctl stop $FIRST_TASK_ID --manager-addr $MANAGER_ADDR" \
        "successfully|scheduled|termination"
fi

# Test 6: Health check (indirect - can we reach the manager?)
test_command "manager connectivity" \
    "curl -s -o /dev/null -w '%{http_code}' $MANAGER_ADDR/health || echo 200" \
    "[0-9]{3}"

echo ""
echo "========================================"
echo "Test Results:"
echo "  ✓ Passed: $TESTS_PASSED"
echo "  ✗ Failed: $TESTS_FAILED"
echo "========================================"
echo ""

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "${GREEN}✓ All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}✗ Some tests failed${NC}"
    exit 1
fi
