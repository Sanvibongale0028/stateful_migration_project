#!/usr/bin/env bash

# Continuous traffic generator for full-flow testing.

GATEWAY_URL="http://localhost:8081"
LOG_FILE="continuous_traffic.log"

CLIENTS=("client101" "client202" "client303" "client404")
RESOURCES=("/api/products" "/api/cart")

request_count=0

echo "===================================================="
echo " FULL FLOW CONTINUOUS TRAFFIC GENERATOR"
echo " Gateway: $GATEWAY_URL"
echo " Backend: http://10.40.17.27:8000"
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
                "$GATEWAY_URL$resource?client_id=$client")

            curl_exit=$?

            response=$(cat "$response_file")
            rm -f "$response_file"

            if [ "$curl_exit" -ne 0 ]; then
                status="NETWORK_ERROR"
                response="curl_exit=$curl_exit"
            fi

            case "$status" in
                200|201|202|204)
                    result="BACKEND_SUCCESS"
                    ;;
                429)
                    result="RATE_LIMITED"
                    ;;
                503)
                    result="NO_ACTIVE_INSTANCE"
                    ;;
                NETWORK_ERROR)
                    result="NETWORK_ERROR"
                    ;;
                *)
                    result="BACKEND_OR_PROXY_ERROR"
                    ;;
            esac

            line="[$timestamp] request=$request_count client=$client resource=$resource status=$status result=$result response=$response"

            echo "$line"
            echo "$line" >> "$LOG_FILE"

        done
    done

    sleep 0.3
done