#!/usr/bin/env bash

set -euo pipefail

REDIS_CONTAINER="redis-state"
CLIENT_ID="continuity-test"
RESOURCE="/api/test"
REDIS_KEY="rate_limit:${CLIENT_ID}:${RESOURCE}"

CURRENT_PRIMARY="http://localhost:4001"
STANDBY="http://localhost:4000"

echo "=========================================="
echo " STATE CONTINUITY FAILOVER TEST"
echo "=========================================="

echo
echo "Step 1: Check current roles"
curl -fsS "${CURRENT_PRIMARY}/role"
echo
curl -fsS "${STANDBY}/role"
echo

echo
echo "Step 2: Send three requests through current Primary"
for i in 1 2 3; do
    echo "Request $i:"
    curl -fsS \
      "${CURRENT_PRIMARY}/check?client_id=${CLIENT_ID}&resource=${RESOURCE}"
    echo
done

echo
echo "Step 3: Read Redis state before failover"
sudo docker exec "${REDIS_CONTAINER}" \
    redis-cli HGETALL "${REDIS_KEY}"

echo
echo "Step 4: Demote current Primary (port 4001)"
curl -fsS -X POST "${CURRENT_PRIMARY}/demote"
echo

echo
echo "Step 5: Promote standby (port 4000)"
curl -fsS -X POST "${STANDBY}/promote"
echo

echo
echo "Step 6: Verify promoted role"
curl -fsS "${STANDBY}/role"
echo

echo
echo "Step 7: Immediately send one request through promoted instance"
curl -fsS \
  "${STANDBY}/check?client_id=${CLIENT_ID}&resource=${RESOURCE}"
echo

echo
echo "Step 8: Read Redis state after failover"
sudo docker exec "${REDIS_CONTAINER}" \
    redis-cli HGETALL "${REDIS_KEY}"

echo
echo "=========================================="
echo " TEST SEQUENCE COMPLETED"
echo "=========================================="
