#!/bin/bash

# ==========================================
# RATE LIMITER FAILURE AND FAILOVER DEMO
# ==========================================

TOTAL_REQUESTS=30
FAIL_AT=8

PRIMARY_CONTAINER="rate-limiter-primary"

API_URL="http://localhost:8081/api/products"

METHOD="GET"

LOG_DIR="demo_logs"
mkdir -p "$LOG_DIR"

LOG_FILE="$LOG_DIR/rate_limiter_demo_$(date +%Y%m%d_%H%M%S).log"

TEMP_FILE=$(mktemp)

# Display output on terminal and save it to a file
exec > >(tee -a "$LOG_FILE") 2>&1

cleanup() {
    rm -f "$TEMP_FILE"
}

trap cleanup EXIT

echo "=========================================="
echo "     RATE LIMITER FAILOVER DEMO"
echo "=========================================="
echo "Total Requests : $TOTAL_REQUESTS"
echo "Failure At     : Request $FAIL_AT"
echo "Primary        : $PRIMARY_CONTAINER"
echo "API Endpoint   : $API_URL"
echo "Log File       : $LOG_FILE"
echo "=========================================="

# ------------------------------------------
# Function to send an individual request
# ------------------------------------------

send_request() {

    local REQUEST_NO=$1
    local START_TIME
    START_TIME=$(date '+%H:%M:%S')

    echo ""
    echo "------------------------------------------"
    echo "Request: $REQUEST_NO / $TOTAL_REQUESTS"
    echo "Time: $START_TIME"
    echo "------------------------------------------"

    local RESPONSE
    local CURL_STATUS

    if RESPONSE=$(curl -sS \
        --max-time 15 \
        -i \
        -X "$METHOD" \
        "$API_URL" 2>&1); then

        CURL_STATUS=0

    else
        CURL_STATUS=$?
    fi

    echo "$RESPONSE"

    if [ "$CURL_STATUS" -eq 0 ]; then
        echo "[RESULT] Request $REQUEST_NO completed."
    else
        echo "[ERROR] Request $REQUEST_NO - curl exit code: $CURL_STATUS"
    fi

    echo "[TIME] $(date '+%H:%M:%S')"

    return 0
}

# ------------------------------------------
# Main Request Execution
# ------------------------------------------

for ((i=1; i<=TOTAL_REQUESTS; i++))
do

    if [ "$i" -eq "$FAIL_AT" ]; then

        echo ""
        echo "=========================================="
        echo " FAILURE TRIGGER AT REQUEST $FAIL_AT"
        echo "=========================================="

        # Start request 8 in background
        send_request "$i" > "$TEMP_FILE" 2>&1 &
        REQUEST_PID=$!

        # Allow the request to reach the gateway
        sleep 0.2

        echo ""
        echo "[FAILURE] Stopping Primary Rate Limiter..."
        date '+Time: %H:%M:%S'

        docker stop "$PRIMARY_CONTAINER"

        if [ $? -eq 0 ]; then
            echo "[SUCCESS] Primary Rate Limiter stopped."
        else
            echo "[ERROR] Could not stop Primary Rate Limiter."
        fi

        # Wait for request 8 to complete
        wait "$REQUEST_PID"
        cat "$TEMP_FILE"

        echo ""
        echo "[RECOVERY] Waiting for Shadow promotion..."
        sleep 2

        echo "[RECOVERY] Continuing requests through Gateway..."

    else

        send_request "$i"

    fi

    sleep 0.2

done

# ------------------------------------------
# Final Summary
# ------------------------------------------

echo ""
echo "=========================================="
echo "          DEMO COMPLETED"
echo "=========================================="

echo "Total Requests Sent : $TOTAL_REQUESTS"
echo "Primary Failure At  : Request $FAIL_AT"
echo "Expected Shadow Requests : $((TOTAL_REQUESTS - FAIL_AT + 1))"

echo ""
echo "Primary Container Status:"
docker ps -a --filter "name=$PRIMARY_CONTAINER"

echo ""
echo "Complete logs saved at:"
echo "$LOG_FILE"

echo "=========================================="