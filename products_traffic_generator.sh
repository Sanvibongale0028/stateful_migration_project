```bash
#!/usr/bin/env bash

GATEWAY_URL="http://localhost:8081"
RESOURCE="/api/products"
LOG_FILE="products_traffic.log"

CLIENTS=("client101" "client202" "client303" "client404")

echo "===================================================="
echo " PRODUCTS ENDPOINT - TRAFFIC GENERATOR"
echo " Gateway: $GATEWAY_URL"
echo " Endpoint: $RESOURCE"
echo " Total Requests: 30"
echo " Log: $LOG_FILE"
echo "===================================================="

for ((request_count=1; request_count<=10; request_count++)); do

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

    echo "$line"
    echo "$line" >> "$LOG_FILE"

    sleep 0.3

done

echo "===================================================="
echo "Completed: 10 requests sent."
echo "Results saved in $LOG_FILE"
echo "===================================================="
```