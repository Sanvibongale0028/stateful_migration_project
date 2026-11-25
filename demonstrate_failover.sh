#!/usr/bin/env bash

# Demonstrate rate-limiter failover and Redis state continuity.
# Requires curl, docker, and redis-cli inside the Redis container.

set -u

PRIMARY_URL="http://localhost:4000"
SHADOW_URL="http://localhost:4001"
GATEWAY_URL="http://localhost:8081"

PRIMARY_CONTAINER="rate-limiter-primary"
SHADOW_CONTAINER="rate-limiter-shadow"
REDIS_CONTAINER="redis-state"

CLIENT_ID="failover-demo-client"
RESOURCE="/api/products"

LOG_FILE="failover_demonstration.log"

exec > >(tee -a "$LOG_FILE") 2>&1

echo
echo "===================================================="
echo " RATE LIMITER FAILOVER DEMONSTRATION"
echo " Started: $(date '+%Y-%m-%d %H:%M:%S')"
echo "===================================================="

# ----------------------------------------------------
# Helper functions
# ----------------------------------------------------

get_role() {
    curl -sS --max-time 2 "$1/role" 2>/dev/null || echo '{"error":"unreachable"}'
}

get_role_name() {
    get_role "$1" |
        grep -o '"role":"[^"]*"' |
        head -n 1 |
        cut -d '"' -f 4
}

show_roles() {
    echo
    echo "---- RATE LIMITER ROLES ----"

    echo "Port 4000:"
    get_role "$PRIMARY_URL"
    echo

    echo "Port 4001:"
    get_role "$SHADOW_URL"
    echo
}

show_redis_state() {
    echo
    echo "---- REDIS RATE-LIMIT STATE ----"

    if ! docker inspect "$REDIS_CONTAINER" >/dev/null 2>&1; then
        echo "ERROR: Redis container '$REDIS_CONTAINER' was not found."
        return 1
    fi

    if ! docker exec "$REDIS_CONTAINER" redis-cli PING; then
        echo "ERROR: Redis is not responding."
        return 1
    fi

    echo "Rate-limit keys:"
    docker exec "$REDIS_CONTAINER" redis-cli --scan \
        --pattern 'rate_limit:*' |
        while IFS= read -r key; do
            [ -z "$key" ] && continue

            echo
            echo "Key: $key"
            docker exec "$REDIS_CONTAINER" redis-cli HGETALL "$key"
            echo "TTL:"
            docker exec "$REDIS_CONTAINER" redis-cli TTL "$key"
        done
}

check_request() {
    local client="$1"
    local resource="$2"

    echo
    echo "Request: client=$client resource=$resource"

    curl -sS \
        --max-time 5 \
        -w '\nHTTP_STATUS=%{http_code}\n' \
        "$GATEWAY_URL/check?client_id=$client&resource=$resource" ||
        echo "REQUEST_ERROR"
}

# ----------------------------------------------------
# Step 1: Check services and roles
# ----------------------------------------------------

echo
echo "[STEP 1] Checking initial roles..."

show_roles

PRIMARY_ROLE=$(get_role_name "$PRIMARY_URL")
SHADOW_ROLE=$(get_role_name "$SHADOW_URL")

if [ "$PRIMARY_ROLE" != "primary" ]; then
    echo "ERROR: Port 4000 is not reporting role=primary."
    echo "No container will be stopped."
    exit 1
fi

if [ "$SHADOW_ROLE" != "shadow" ]; then
    echo "ERROR: Port 4001 is not reporting role=shadow."
    echo "No container will be stopped."
    exit 1
fi

if ! docker inspect "$PRIMARY_CONTAINER" >/dev/null 2>&1; then
    echo "ERROR: Primary container '$PRIMARY_CONTAINER' was not found."
    echo "No container will be stopped."
    exit 1
fi

if ! docker inspect "$SHADOW_CONTAINER" >/dev/null 2>&1; then
    echo "ERROR: Shadow container '$SHADOW_CONTAINER' was not found."
    echo "No container will be stopped."
    exit 1
fi

# ----------------------------------------------------
# Step 2: Establish initial state for a known client
# ----------------------------------------------------

echo
echo "[STEP 2] Establishing initial state..."

for i in 1 2 3; do
    check_request "$CLIENT_ID" "$RESOURCE"
    sleep 0.2
done

echo
echo "Redis state before failure:"
show_redis_state

# ----------------------------------------------------
# Step 3: Stop Primary
# ----------------------------------------------------

echo
echo "[STEP 3] Primary container failure..."
echo "Stopping container: $PRIMARY_CONTAINER"
echo "Timestamp: $(date '+%Y-%m-%d %H:%M:%S.%3N')"

docker stop "$PRIMARY_CONTAINER"

echo
echo "Primary container status:"
docker ps -a --filter "name=^/${PRIMARY_CONTAINER}$" \
    --format 'Name={{.Names}} Status={{.Status}}'

# ----------------------------------------------------
# Step 4: Wait for automatic promotion
# ----------------------------------------------------

echo
echo "[STEP 4] Waiting for automatic Shadow promotion..."

PROMOTED=false

for attempt in $(seq 1 40); do
    role=$(get_role_name "$SHADOW_URL")

    echo "Attempt $attempt/40: port 4001 role=$role"

    if [ "$role" = "primary" ]; then
        PROMOTED=true
        break
    fi

    sleep 0.5
done

if [ "$PROMOTED" != true ]; then
    echo
    echo "FAIL: Shadow did not become Primary within 20 seconds."
    echo "Inspect Failure Detector and Recovery Manager logs."
    show_roles
    exit 1
fi

echo
echo "SUCCESS: Shadow promotion detected."
echo "Promotion timestamp: $(date '+%Y-%m-%d %H:%M:%S.%3N')"

# ----------------------------------------------------
# Step 5: Verify roles and container status
# ----------------------------------------------------

echo
echo "[STEP 5] Verifying roles..."

show_roles

echo "Former Primary container:"
docker ps -a --filter "name=^/${PRIMARY_CONTAINER}$" \
    --format 'Name={{.Names}} Status={{.Status}}'

echo "Former Shadow container:"
docker ps -a --filter "name=^/${SHADOW_CONTAINER}$" \
    --format 'Name={{.Names}} Status={{.Status}}'

# ----------------------------------------------------
# Step 6: Compare Redis state after promotion
# ----------------------------------------------------

echo
echo "[STEP 6] Redis state after failover..."

show_redis_state

# ----------------------------------------------------
# Step 7: Test requests after failover
# ----------------------------------------------------

echo
echo "[STEP 7] Sending requests after promotion..."

for i in 1 2 3 4 5; do
    check_request "$CLIENT_ID" "$RESOURCE"
    sleep 0.2
done

# Test another client to demonstrate client isolation.
check_request "failover-demo-client-2" "/api/cart"

# ----------------------------------------------------
# Step 8: Final summary
# ----------------------------------------------------

echo
echo "===================================================="
echo " DEMONSTRATION SUMMARY"
echo "===================================================="

echo "Former Primary role:"
get_role "$PRIMARY_URL"
echo

echo "Former Shadow role:"
get_role "$SHADOW_URL"
echo

echo "Redis state after failover:"
show_redis_state

echo
echo "Gateway test:"
curl -sS --max-time 5 \
    -w '\nHTTP_STATUS=%{http_code}\n' \
    "$GATEWAY_URL/check?client_id=final-verification&resource=/api/products" ||
    echo "Gateway request failed"

echo
echo "Finished: $(date '+%Y-%m-%d %H:%M:%S')"
echo "Full demonstration log: $LOG_FILE"
echo "===================================================="
