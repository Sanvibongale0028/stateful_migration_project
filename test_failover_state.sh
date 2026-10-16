#!/bin/bash

set -u

PRIMARY_PORT=4000
SECONDARY_PORT=4001

PRIMARY_ID="primary-test"
SECONDARY_ID="secondary-test"

CLIENT_ID="failover-test"
RESOURCE="/api/products"

KEY="rate_limit:${CLIENT_ID}:${RESOURCE}"

echo "========================================"
echo " STATEFUL FAILOVER TEST"
echo "========================================"

# ----------------------------------------
# 1. Clean previous test state
# ----------------------------------------

echo
echo "[1] Cleaning previous Redis state..."

docker exec redis-state redis-cli DEL "$KEY"

# ----------------------------------------
# 2. Start Primary
# ----------------------------------------

echo
echo "[2] Starting Primary on :$PRIMARY_PORT..."

RATE_LIMITER_PORT=$PRIMARY_PORT \
RATE_LIMITER_ROLE=primary \
RATE_LIMITER_INSTANCE_ID=$PRIMARY_ID \
RATE_LIMIT_REFILL_RATE=0.1 \
go run ./cmd/server > primary-test.log 2>&1 &

PRIMARY_PID=$!

echo "Primary PID: $PRIMARY_PID"

sleep 3

# ----------------------------------------
# 3. Start Secondary
# ----------------------------------------

echo
echo "[3] Starting Secondary on :$SECONDARY_PORT..."

RATE_LIMITER_PORT=$SECONDARY_PORT \
RATE_LIMITER_ROLE=shadow \
RATE_LIMITER_INSTANCE_ID=$SECONDARY_ID \
RATE_LIMIT_REFILL_RATE=0.1 \
go run ./cmd/server > secondary-test.log 2>&1 &

SECONDARY_PID=$!

echo "Secondary PID: $SECONDARY_PID"

sleep 3

# ----------------------------------------
# 4. Check roles
# ----------------------------------------

echo
echo "[4] Current roles..."

echo "Primary:"
curl -s "http://localhost:$PRIMARY_PORT/role"

echo

echo "Secondary:"
curl -s "http://localhost:$SECONDARY_PORT/role"

echo

# ----------------------------------------
# 5. Consume 5 tokens
# ----------------------------------------

echo
echo "[5] Consuming 5 tokens from Primary..."

for i in {1..5}
do
    echo "Request $i:"
    curl -s \
        "http://localhost:$PRIMARY_PORT$RESOURCE?client_id=$CLIENT_ID"

    echo
done

# ----------------------------------------
# 6. Check Redis state
# ----------------------------------------

echo
echo "[6] Redis state BEFORE failure..."

docker exec redis-state redis-cli HGETALL "$KEY"

# ----------------------------------------
# 7. Stop Primary
# ----------------------------------------

echo
echo "[7] Simulating Primary failure..."

kill "$PRIMARY_PID"

echo "Primary stopped."

sleep 2

# ----------------------------------------
# 8. Promote Secondary
# ----------------------------------------

echo
echo "[8] Promoting Secondary..."

PROMOTE_RESPONSE=$(curl -s -X POST \
    "http://localhost:$SECONDARY_PORT/promote")

echo "$PROMOTE_RESPONSE"

sleep 1

# ----------------------------------------
# 9. Check Secondary role
# ----------------------------------------

echo
echo "[9] Secondary role after promotion..."

curl -s "http://localhost:$SECONDARY_PORT/role"

echo

# ----------------------------------------
# 10. Send request after failover
# ----------------------------------------

echo
echo "[10] Sending request through promoted Secondary..."

curl -i \
    "http://localhost:$SECONDARY_PORT$RESOURCE?client_id=$CLIENT_ID"

# ----------------------------------------
# 11. Check Redis state
# ----------------------------------------

echo
echo
echo "[11] Redis state AFTER failover..."

docker exec redis-state redis-cli HGETALL "$KEY"

# ----------------------------------------
# 12. Cleanup
# ----------------------------------------

echo
echo "[12] Cleaning up..."

kill "$SECONDARY_PID" 2>/dev/null || true

echo
echo "========================================"
echo " TEST COMPLETE"
echo "========================================"