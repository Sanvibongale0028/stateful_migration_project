#!/usr/bin/env bash

# Continuous traffic generator for rate-limiter failover demonstration.

GATEWAY_URL="http://localhost:8081"
LOG_FILE="continuous_traffic.log"

CLIENTS=("client101" "client202" "client303" "client404")
RESOURCES=("/api/products" "/api/cart")

request_count=0

echo "===================================================="
echo " CONTINUOUS TRAFFIC GENERATOR"
echo " Gateway: $GATEWAY_URL"
echo " Log:     $LOG_FILE"
echo " Press Ctrl+C to stop"
echo "===================================================="

while true; do
    for client in "${CLIENTS[@]}"; do
        for resource in "${RESOURCES[@]}"; do
            request_count=$((request_count + 1))
            timestamp=$(date '+%Y-%m-%d %H:%M:%S')

            response_file=$(mktemp)

            status=$(curl -sS \
                --connect-timeout 2 \
                --max-time 5 \
                -o "$response_file" \
                -w '%{http_code}' \
                "$GATEWAY_URL/check?client_id=$client&resource=$resource")

            curl_exit=$?

            response=$(cat "$response_file")
            rm -f "$response_file"

            if [ "$curl_exit" -ne 0 ]; then
                status="NETWORK_ERROR"
                response="curl_exit=$curl_exit"
            fi

            case "$status" in
                200)
                    result="ACCEPTED"
                    ;;
                429)
                    result="RATE_LIMITED"
                    ;;
                503)
                    result="NO_ACTIVE_INSTANCE"
                    ;;
                *)
                    result="ERROR"
                    ;;
            esac

            line="[$timestamp] request=$request_count client=$client resource=$resource status=$status result=$result response=$response"

            echo "$line"
            echo "$line" >> "$LOG_FILE"
        done
    done

    sleep 0.3
done
