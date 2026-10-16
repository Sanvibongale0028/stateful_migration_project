#!/bin/bash

PRIMARY="http://localhost:4000"
SECONDARY="http://localhost:4001"

CLIENT="test101"
RESOURCE="/api/test"

REDIS_CONTAINER="redis-state"
REDIS_KEY="rate_limit:${CLIENT}:${RESOURCE}"

echo "========================================"
echo " STATEFUL RATE LIMITER FAILOVER TEST"
echo "========================================"

# --------------------------------------------------
# 1. Clean previous Redis state
# --------------------------------------------------

echo
echo "[1] Cleaning previous Redis state..."

docker exec "$REDIS_CONTAINER" redis-cli DEL "$REDIS_KEY"

echo

# --------------------------------------------------
# 2. Check initial roles
# --------------------------------------------------

echo "[2] Checking initial roles..."

echo
echo "Primary:"
curl -s "$PRIMARY/role"
echo

echo "Secondary:"
curl -s "$SECONDARY/role"
echo

# --------------------------------------------------
# 3. Check Redis primary ownership
# --------------------------------------------------

echo
echo "[3] Checking Redis primary ownership..."

docker exec "$REDIS_CONTAINER" \
    redis-cli GET rate_limiter:primary

echo

# --------------------------------------------------
# 4. Consume tokens from Primary
# --------------------------------------------------

echo
echo "[4] Consuming 5 tokens from Primary..."
echo

for i in {1..5}
do
    echo "Request $i:"

    curl -s \
        "$PRIMARY/check?client_id=$CLIENT&resource=$RESOURCE"

    echo
done

# --------------------------------------------------
# 5. Check Redis state before failure
# --------------------------------------------------

echo
echo "[5] Redis state BEFORE Primary failure..."

docker exec "$REDIS_CONTAINER" \
    redis-cli HGETALL "$REDIS_KEY"

echo

# --------------------------------------------------
# 6. Fail Primary manually
# --------------------------------------------------

echo "========================================"
echo " NOW FAIL THE PRIMARY"
echo "========================================"

echo
echo "Go to the Primary terminal and press:"
echo
echo "    Ctrl+C"
echo
echo "Then wait for:"
echo
echo "    Failure Detector"
echo "          ↓"
echo "    Recovery Manager"
echo "          ↓"
echo "    Secondary promotion"
echo
echo "Wait until the Secondary reports:"
echo
echo '    {"active":true,"role":"primary"}'
echo

read -p "Press ENTER after automatic failover has completed..."

# --------------------------------------------------
# 7. Check Secondary role
# --------------------------------------------------

echo
echo "[6] Checking Secondary role..."

ROLE_RESPONSE=$(curl -s "$SECONDARY/role")

echo "$ROLE_RESPONSE"

echo

# --------------------------------------------------
# 8. Verify Secondary is actually Primary
# --------------------------------------------------

if echo "$ROLE_RESPONSE" | grep -q '"role":"primary"'; then
    echo "SUCCESS: Secondary is now Primary."
else
    echo "ERROR: Secondary was not promoted."
    echo
    echo "Current role:"
    echo "$ROLE_RESPONSE"
    exit 1
fi

# --------------------------------------------------
# 9. Check Redis state before Secondary request
# --------------------------------------------------

echo
echo "[7] Redis state BEFORE Secondary request..."

docker exec "$REDIS_CONTAINER" \
    redis-cli HGETALL "$REDIS_KEY"

echo

# --------------------------------------------------
# 10. Send one request through promoted Secondary
# --------------------------------------------------

echo "[8] Sending ONE request to promoted Secondary..."

curl -i \
    "$SECONDARY/check?client_id=$CLIENT&resource=$RESOURCE"

echo

# --------------------------------------------------
# 11. Check Redis state after Secondary request
# --------------------------------------------------

echo
echo "[9] Redis state AFTER Secondary request..."

docker exec "$REDIS_CONTAINER" \
    redis-cli HGETALL "$REDIS_KEY"

echo

# --------------------------------------------------
# 12. Final result
# --------------------------------------------------

echo "========================================"
echo " TEST COMPLETE"
echo "========================================"

echo
echo "Expected behavior:"
echo
echo "1. Primary consumes 5 tokens."
echo "2. Redis state becomes approximately 0."
echo "3. Primary fails."
echo "4. Failure Detector detects failure."
echo "5. Recovery Manager promotes Secondary."
echo "6. Secondary becomes Primary."
echo "7. Secondary reads the SAME Redis bucket."
echo "8. Secondary consumes one token from that existing state."
echo