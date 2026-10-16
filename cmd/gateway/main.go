package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
	"os"
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
		log.Printf("Cannot reach %s: %v", baseURL, err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	var role RoleResponse

	if err := json.NewDecoder(resp.Body).Decode(&role); err != nil {
		log.Printf("Invalid role response from %s: %v", baseURL, err)
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
) error {
	target, err := url.Parse(activeURL)
	if err != nil {
		log.Printf("Invalid rate-limiter URL: %v", err)
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

	log.Printf(
		"Forwarding %s %s to %s",
		r.Method,
		strings.TrimSpace(r.URL.RequestURI()),
		activeURL,
	)

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
		log.Printf("Error copying response from %s: %v", activeURL, err)
	}

	return nil
}

// Forward to the selected instance and attempt one fallback on a network error.
func gatewayHandler(w http.ResponseWriter, r *http.Request) {
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
		log.Printf("Selected instance %s is no longer active", activeURL)

		if !isActive(otherURL) {
			http.Error(
				w,
				"No active rate limiter available",
				http.StatusServiceUnavailable,
			)
			return
		}

		log.Printf("Falling back to %s", otherURL)
		activeURL = otherURL
		otherURL = primaryURL
		if activeURL == primaryURL {
			otherURL = secondaryURL
		}
	}

	// Attempt the selected instance first.
	err := forwardRequest(w, r, activeURL)
	if err == nil {
		return
	}

	log.Printf("Forwarding to %s failed: %v", activeURL, err)

	// Only attempt one fallback, and only if it currently reports ACTIVE.
	if !isActive(otherURL) {
		http.Error(
			w,
			"No active rate limiter available",
			http.StatusServiceUnavailable,
		)
		return
	}

	log.Printf("Retrying request with %s", otherURL)

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

	if retryErr := forwardRequest(w, r, otherURL); retryErr != nil {
		log.Printf("Retry to %s failed: %v", otherURL, retryErr)
		http.Error(
			w,
			"Forwarding failed on both rate-limiter instances",
			http.StatusBadGateway,
		)
	}
}

func main() {
	http.HandleFunc("/", gatewayHandler)

	log.Printf("Gateway listening on %s", gatewayPort)

	if err := http.ListenAndServe(gatewayPort, nil); err != nil {
		log.Fatalf("Gateway stopped: %v", err)
	}
}
