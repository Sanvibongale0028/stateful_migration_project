```bash
#!/usr/bin/env bash

GATEWAY_URL="http://localhost:8081"
RESOURCE="/api/products"
TOTAL_REQUESTS=30

CLIENTS=("client101" "client202" "client303" "client404")

# Fixed log filename
LOG_FILE="products_traffic.log"

# Create and clear log file immediately
> "$LOG_FILE"

# Write initial header
{
    echo "===================================================="
    echo "PRODUCT ENDPOINT TRAFFIC LOG"
    echo "Gateway: $GATEWAY_URL"
    echo "Endpoint: $RESOURCE"
    echo "Total Requests: $TOTAL_REQUESTS"
    echo "Started: $(date '+%Y-%m-%d %H:%M:%S')"
    echo "===================================================="
} | tee -a "$LOG_FILE"

for ((request_count=1; request_count<=TOTAL_REQUESTS; request_count++))
do
    client=${CLIENTS[$(( (request_count - 1) % ${#CLIENTS[@]} ))]}
    timestamp=$(date '+%Y-%m-%d %H:%M:%S')

    response=$(curl -sS \
        --connect-timeout 2 \
        --max-time 5 \
        -w '\nHTTP_STATUS:%{http_code}' \
        "$GATEWAY_URL$RESOURCE?client_id=$client" 2>&1)

    curl_exit=$?

    if [ "$curl_exit" -ne 0 ]; then
        status="NETWORK_ERROR"
        result="REQUEST_FAILED"
        response="curl_exit=$curl_exit $response"
    else
        status=$(echo "$response" | sed -n 's/.*HTTP_STATUS://p' | tail -1)
        response=$(echo "$response" | sed '/HTTP_STATUS:/d')

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
            *)
                result="BACKEND_OR_PROXY_ERROR"
                ;;
        esac
    fi

    # Generate and save each request log immediately
    printf '[%s] request=%02d/%02d client=%s endpoint=%s status=%s result=%s response=%s\n' \
        "$timestamp" "$request_count" "$TOTAL_REQUESTS" \
        "$client" "$RESOURCE" "$status" "$result" "$response" \
        | tee -a "$LOG_FILE"

    sleep 0.1

done

{
    echo "===================================================="
    echo "Completed: $TOTAL_REQUESTS requests"
    echo "Finished: $(date '+%Y-%m-%d %H:%M:%S')"
    echo "Log saved in: $LOG_FILE"
    echo "===================================================="
} | tee -a "$LOG_FILE"
```