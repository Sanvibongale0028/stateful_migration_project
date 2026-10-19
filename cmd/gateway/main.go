package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	primaryURL   = getEnv("PRIMARY_URL", "http://localhost:4000")
	secondaryURL = getEnv("SECONDARY_URL", "http://localhost:4001")
	gatewayPort  = ":" + getEnv("GATEWAY_PORT", "8081")
)

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func logEvent(eventType, level, message string, fields map[string]string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")

	var details strings.Builder

	for key, value := range fields {
		fmt.Fprintf(&details, " %s=%q", key, value)
	}

	log.Printf(
		"%s | GATEWAY | rate-limiter-gateway | %s | %s | %s%s",
		timestamp,
		eventType,
		level,
		message,
		details.String(),
	)
}

var httpClient = &http.Client{
	Timeout: 2 * time.Second,
}

type RoleResponse struct {
	Role   string `json:"role"`
	Active bool   `json:"active"`
}

// Check whether a rate-limiter instance is active.
func isActive(baseURL string) bool {
	roleURL := baseURL + "/role"

	resp, err := httpClient.Get(roleURL)
	if err != nil {
		// log.Printf("Cannot reach %s: %v", baseURL, err)
		logEvent("upstream_health_check_failed", "WARN", "Cannot reach rate limiter", map[string]string{
			"target": baseURL,
			"error":  err.Error(),
		})
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	var role RoleResponse

	if err := json.NewDecoder(resp.Body).Decode(&role); err != nil {
		// log.Printf("Invalid role response from %s: %v", baseURL, err)
		logEvent("invalid_role_response", "ERROR", "Invalid rate limiter role response", map[string]string{
			"target": baseURL,
			"error":  err.Error(),
		})
		return false
	}

	return role.Active && role.Role == "primary"
}

// Find the active rate-limiter instance.
func getActiveRateLimiter() string {
	if isActive(primaryURL) {
		return primaryURL
	}

	if isActive(secondaryURL) {
		return secondaryURL
	}

	return ""
}

// Forward a request to a rate-limiter instance.
// Returns an error if the network request fails before a response is received.
func forwardRequest(
	w http.ResponseWriter,
	r *http.Request,
	activeURL string,
	reqID string,
) error {
	target, err := url.Parse(activeURL)
	if err != nil {
		// log.Printf("Invalid rate-limiter URL: %v", err)
		logEvent("invalid_upstream_url", "ERROR", "Invalid rate limiter URL", map[string]string{
			"error": err.Error(),
		})
		http.Error(w, "Gateway configuration error", http.StatusInternalServerError)
		return nil
	}

	// Build the upstream request while preserving the original path and query.
	upstreamURL := *target
	upstreamURL.Path = r.URL.Path
	upstreamURL.RawPath = r.URL.RawPath
	upstreamURL.RawQuery = r.URL.RawQuery

	upstreamRequest, err := http.NewRequestWithContext(
		r.Context(),
		r.Method,
		upstreamURL.String(),
		r.Body,
	)
	if err != nil {
		http.Error(w, "Could not create upstream request", http.StatusInternalServerError)
		return nil
	}

	// Preserve request headers.
	upstreamRequest.Header = r.Header.Clone()
	upstreamRequest.Host = target.Host
	upstreamRequest.ContentLength = r.ContentLength

	// log.Printf(
	// 	"Forwarding %s %s to %s",
	// 	r.Method,
	// 	strings.TrimSpace(r.URL.RequestURI()),
	// 	activeURL,
	// )

	logEvent("request_forwarded", "INFO", "Forwarding request to rate limiter", map[string]string{
		"req_id": reqID,
		"from":   "gateway",
		"target": activeURL,
		"method": r.Method,
		"path":   r.URL.Path,
	})

	// Do not retry automatically after receiving an HTTP response.
	resp, err := httpClient.Do(upstreamRequest)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Copy response headers, status and body to the client.
	for name, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}

	w.WriteHeader(resp.StatusCode)

	if _, err := io.Copy(w, resp.Body); err != nil {
		// log.Printf("Error copying response from %s: %v", activeURL, err)
		logEvent("response_copy_failed", "ERROR", "Failed to copy upstream response", map[string]string{
			"target": activeURL,
			"error":  err.Error(),
		})
	}

	return nil
}

// Forward to the selected instance and attempt one fallback on a network error.
func gatewayHandler(w http.ResponseWriter, r *http.Request) {
	reqID := uuid.New().String()

	logEvent("request_received", "INFO", "Request received by gateway", map[string]string{
		"req_id": reqID,
		"from":   "client",
		"method": r.Method,
		"path":   r.URL.Path,
	})

	activeURL := getActiveRateLimiter()

	if activeURL == "" {
		http.Error(
			w,
			"No active rate limiter available",
			http.StatusServiceUnavailable,
		)
		return
	}

	otherURL := primaryURL
	if activeURL == primaryURL {
		otherURL = secondaryURL
	}

	// Recheck the selected instance before forwarding.
	if !isActive(activeURL) {
		// log.Printf("Selected instance %s is no longer active", activeURL)
		logEvent("upstream_unavailable", "WARN", "Selected instance is no longer active", map[string]string{
			"req_id": reqID,
			"from":   activeURL,
		})

		if !isActive(otherURL) {
			http.Error(
				w,
				"No active rate limiter available",
				http.StatusServiceUnavailable,
			)
			return
		}

		// log.Printf("Falling back to %s", otherURL)
		logEvent("upstream_switched", "WARN", "Falling back to another rate limiter", map[string]string{
			"req_id": reqID,
			"from":   activeURL,
			"target": otherURL,
		})
		activeURL = otherURL
		otherURL = primaryURL
		if activeURL == primaryURL {
			otherURL = secondaryURL
		}
	}

	// Attempt the selected instance first.
	// err := forwardRequest(w, r, activeURL)
	err := forwardRequest(w, r, activeURL, reqID)
	if err == nil {
		return
	}

	// log.Printf("Forwarding to %s failed: %v", activeURL, err)
	logEvent("request_failed", "ERROR", "Forwarding request failed", map[string]string{
		"req_id": reqID,
		"target": activeURL,
		"error":  err.Error(),
	})

	// Only attempt one fallback, and only if it currently reports ACTIVE.
	if !isActive(otherURL) {
		http.Error(
			w,
			"No active rate limiter available",
			http.StatusServiceUnavailable,
		)
		return
	}

	// log.Printf("Retrying request with %s", otherURL)
	logEvent("request_retry", "WARN", "Retrying request with alternate rate limiter", map[string]string{
		"req_id": reqID,
		"from":   activeURL,
		"target": otherURL,
	})

	// A request body may have been consumed by the first attempt.
	// Only retry bodyless requests to avoid sending an incomplete body.
	if r.Body != nil && r.ContentLength != 0 {
		http.Error(
			w,
			"Forwarding failed; request was not retried because it may contain a body",
			http.StatusBadGateway,
		)
		return
	}

	// if retryErr := forwardRequest(w, r, otherURL); retryErr != nil {
	if retryErr := forwardRequest(w, r, otherURL, reqID); retryErr != nil {
		// log.Printf("Retry to %s failed: %v", otherURL, retryErr)
		logEvent("retry_failed", "ERROR", "Retry to alternate rate limiter failed", map[string]string{
			"req_id": reqID,
			"target": otherURL,
			"error":  retryErr.Error(),
		})
		http.Error(
			w,
			"Forwarding failed on both rate-limiter instances",
			http.StatusBadGateway,
		)
	}
}

func main() {
	http.HandleFunc("/", gatewayHandler)

	// log.Printf("Gateway listening on %s", gatewayPort)
	logEvent("gateway_started", "INFO", "Gateway listening", map[string]string{
		"port": gatewayPort,
	})

	if err := http.ListenAndServe(gatewayPort, nil); err != nil {
		log.Fatalf("Gateway stopped: %v", err)
	}
}
