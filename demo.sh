#!/usr/bin/env bash

GATEWAY_URL="http://localhost:8081"
RESOURCE="/api/products"
TOTAL_REQUESTS=30

CLIENTS=("client101" "client202" "client303" "client404")

# Hardcoded log file
touch products_traffic.log
: > products_traffic.log

echo "===================================================="
echo " PRODUCTS ENDPOINT - CONTINUOUS TRAFFIC GENERATOR"
echo " Gateway: $GATEWAY_URL"
echo " Endpoint: $RESOURCE"
echo " Total Requests: $TOTAL_REQUESTS"
echo " Log: products_traffic.log"
echo "===================================================="

for ((request_count=1; request_count<=TOTAL_REQUESTS; request_count++)); do

    client=${CLIENTS[$(( (request_count - 1) % ${#CLIENTS[@]} ))]}
    timestamp=$(date '+%Y-%m-%d %H:%M:%S')

    response_file=$(mktemp)

    status=$(curl -sS \
        --connect-timeout 2 \
        --max-time 5 \
        -o "$response_file" \
        -w '%{http_code}' \
        "$GATEWAY_URL$RESOURCE?client_id=$client")

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

    line="[$timestamp] request=$request_count client=$client resource=$RESOURCE status=$status result=$result response=$response"

    echo "$line" | tee -a products_traffic.log

    sleep 0.1

done

echo "===================================================="
echo "Completed: $TOTAL_REQUESTS requests sent."
echo "Results saved in products_traffic.log"
echo "===================================================="