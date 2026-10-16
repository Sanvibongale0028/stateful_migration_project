package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	primaryURL   = "http://localhost:4000"
	secondaryURL = "http://localhost:4001"
	clientID     = "downtime-test-2"
	resource     = "/api/test"
)

type RoleResponse struct {
	Role   string `json:"role"`
	Active bool   `json:"active"`
}

type CheckResponse struct {
	Allowed   bool `json:"allowed"`
	Remaining int  `json:"remaining"`
}

type RequestResult struct {
	StatusCode   int
	ResponseTime time.Duration
	Err          error
}

var client = &http.Client{
	Timeout: 1 * time.Second,
}

func checkRole(baseURL string) (RoleResponse, error) {
	var role RoleResponse

	resp, err := client.Get(baseURL + "/role")
	if err != nil {
		return role, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return role, fmt.Errorf("role endpoint returned HTTP %d", resp.StatusCode)
	}

	err = json.NewDecoder(resp.Body).Decode(&role)
	return role, err
}

func sendCheck(baseURL string) RequestResult {
	start := time.Now()

	url := fmt.Sprintf(
		"%s/check?client_id=%s&resource=%s",
		baseURL,
		clientID,
		resource,
	)

	resp, err := client.Get(url)
	elapsed := time.Since(start)

	if err != nil {
		return RequestResult{
			ResponseTime: elapsed,
			Err:          err,
		}
	}
	defer resp.Body.Close()

	return RequestResult{
		StatusCode:   resp.StatusCode,
		ResponseTime: elapsed,
	}
}

func main() {
	fmt.Println("=== Docker Rate Limiter Downtime Test ===")
	fmt.Println("Primary:", primaryURL)
	fmt.Println("Secondary:", secondaryURL)
	fmt.Println("Press Ctrl+C to stop.")
	fmt.Println()

	var firstFailure time.Time
	var recoveryRecorded bool

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		result := sendCheck(primaryURL)
		now := time.Now()

		if result.Err == nil && result.StatusCode == http.StatusTooManyRequests {
			fmt.Printf(
				"RATE LIMITED | time=%s | latency=%s\n",
				now.Format(time.RFC3339Nano),
				result.ResponseTime,
			)
			continue
		}

		if result.Err == nil && result.StatusCode == http.StatusOK {
			if firstFailure.IsZero() {
				fmt.Printf(
					"PRIMARY OK | status=%d | latency=%s\n",
					result.StatusCode,
					result.ResponseTime,
				)
			}
			continue
		}

		if firstFailure.IsZero() {
			firstFailure = now
			fmt.Printf(
				"\nFIRST FAILURE | time=%s | status=%d | error=%v\n",
				firstFailure.Format(time.RFC3339Nano),
				result.StatusCode,
				result.Err,
			)
		}

		role, err := checkRole(secondaryURL)
		if err != nil || !role.Active || role.Role != "primary" {
			fmt.Printf(
				"SECONDARY NOT ACTIVE | time=%s | role=%q | active=%t | error=%v\n",
				now.Format(time.RFC3339Nano),
				role.Role,
				role.Active,
				err,
			)
			continue
		}

		secondaryResult := sendCheck(secondaryURL)

		if secondaryResult.Err == nil &&
			(secondaryResult.StatusCode == http.StatusOK || secondaryResult.StatusCode == http.StatusTooManyRequests) {
			if !recoveryRecorded {
				recoveryRecorded = true
				recoveryTime := time.Since(firstFailure)

				fmt.Printf(
					"\nRECOVERY OBSERVED | time=%s | status=%d | latency=%s\n",
					time.Now().Format(time.RFC3339Nano),
					secondaryResult.StatusCode,
					secondaryResult.ResponseTime,
				)
				fmt.Printf(
					"OBSERVED FAILOVER INTERVAL: %s (%.3f seconds)\n\n",
					recoveryTime,
					recoveryTime.Seconds(),
				)
			}
		} else {
			fmt.Printf(
				"SECONDARY ACTIVE BUT CHECK FAILED | status=%d | latency=%s | error=%v\n",
				secondaryResult.StatusCode,
				secondaryResult.ResponseTime,
				secondaryResult.Err,
			)
		}
	}
}
