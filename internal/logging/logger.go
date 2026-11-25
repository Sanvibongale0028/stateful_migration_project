package logging

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

var mu sync.Mutex

var validLayers = map[string]bool{
	"CLIENT":       true,
	"RATE-LIMITER": true,
	"NGINX":        true,
	"BACKEND":      true,
	"PGBOUNCER":    true,
	"DB":           true,
	"FAILOVER":     true,
	"SYSTEM":       true,
}

var validEvents = map[string]bool{
	"request_sent":       true,
	"request_received":   true,
	"request_forwarded":  true,
	"db_query":           true,
	"query_forwarded":    true,
	"query_executed":     true,
	"response_sent":      true,
	"response_received":  true,
	"threshold_reached":  true,
	"failover_started":   true,
	"failover_complete":  true,
	"upstream_switched":  true,
	"db_reconnected":     true,
	"shutting_down":      true,
	"service_recovered":  true,
	"promotion_attempt":  true,
	"promotion_retry":    true,
	"promotion_accepted": true,
	"promotion_verified": true,
}

var validLevels = map[string]bool{
	"INFO":     true,
	"WARN":     true,
	"ERROR":    true,
	"CRITICAL": true,
}

func Log(
	layer string,
	instance string,
	event string,
	level string,
	message string,
	fields map[string]string,
) error {

	// Validate fixed fields.
	if !validLayers[layer] {
		return fmt.Errorf("invalid log layer: %s", layer)
	}

	if !validEvents[event] {
		return fmt.Errorf("invalid event type: %s", event)
	}

	if !validLevels[level] {
		return fmt.Errorf("invalid log level: %s", level)
	}

	if instance == "" || message == "" {
		return fmt.Errorf("instance and message cannot be empty")
	}

	// Generate timestamp with milliseconds.
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")

	// Sort metadata keys for consistent output.
	keys := make([]string, 0, len(fields))

	for key := range fields {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	var metadata strings.Builder

	for _, key := range keys {
		value := fields[key]

		if key == "" {
			return fmt.Errorf("metadata key cannot be empty")
		}

		// Keep each log entry on one line.
		value = strings.ReplaceAll(value, "|", "%7C")
		value = strings.ReplaceAll(value, "\n", "_")
		value = strings.ReplaceAll(value, "\r", "_")
		value = strings.ReplaceAll(value, " ", "_")

		if metadata.Len() > 0 {
			metadata.WriteString(" ")
		}

		fmt.Fprintf(&metadata, "%s=%s", key, value)
	}

	// Print the complete log line atomically.
	mu.Lock()
	defer mu.Unlock()

	fmt.Fprintf(
		os.Stdout,
		"%s | %s | %s | %s | %s | %s | %s\n",
		timestamp,
		layer,
		instance,
		event,
		level,
		message,
		metadata.String(),
	)

	return nil
}
