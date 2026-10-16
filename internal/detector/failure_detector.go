package detector

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"bytes"
	"ecommerce-rate-limiter/internal/events"
	"encoding/json"
)

type State string

type FailureReason string

const (
	ProcessFailure FailureReason = "PROCESS_FAILURE"
	Unresponsive   FailureReason = "UNRESPONSIVE"
)

const (
	Healthy   State = "HEALTHY"
	Suspected State = "SUSPECTED"
	Failed    State = "FAILED"
)

type FailureDetector struct {
	TargetURL        string
	RecoveryURL      string
	CheckInterval    time.Duration
	RequestTimeout   time.Duration
	FailureThreshold int

	InstanceID string
	MachineID  string

	EventReporter *events.Reporter

	client         *http.Client
	recoveryClient *http.Client
}

func NewFailureDetector(
	targetURL string,
	recoveryURL string,
	checkInterval time.Duration,
	requestTimeout time.Duration,
	failureThreshold int,
	instanceID string,
	machineID string,
	eventReporter *events.Reporter,
) *FailureDetector {

	return &FailureDetector{
		TargetURL:        targetURL,
		RecoveryURL:      recoveryURL,
		CheckInterval:    checkInterval,
		RequestTimeout:   requestTimeout,
		FailureThreshold: failureThreshold,

		InstanceID: instanceID,
		MachineID:  machineID,

		EventReporter: eventReporter,

		client: &http.Client{
			Timeout: requestTimeout,
		},

		recoveryClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// func (fd *FailureDetector) checkHealth() (bool, FailureReason) {

// 	resp, err := fd.client.Get(fd.TargetURL)

// 	if err != nil {

// 		if err, ok := err.(net.Error); ok && err.Timeout() {
// 			return false, Unresponsive
// 		}

// 		return false, ProcessFailure
// 	}

// 	defer resp.Body.Close()

// 	if resp.StatusCode != http.StatusOK {
// 		return false, Unresponsive
// 	}

// 	return true, ""
// }

func (fd *FailureDetector) checkHealth() (bool, FailureReason, int64) {

	start := time.Now()

	resp, err := fd.client.Get(fd.TargetURL)

	responseTimeMS := time.Since(start).Milliseconds()

	if err != nil {

		if err, ok := err.(net.Error); ok && err.Timeout() {
			return false, Unresponsive, responseTimeMS
		}

		return false, ProcessFailure, responseTimeMS
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, Unresponsive, responseTimeMS
	}

	return true, "", responseTimeMS
}

func (fd *FailureDetector) sendFailureEvent(
	reason FailureReason,
	consecutiveFailures int,
) {

	// Notify Recovery Manager
	event := map[string]interface{}{
		"service":              "rate-limiter",
		"failure_type":         string(reason),
		"detected_at":          time.Now(),
		"consecutive_failures": consecutiveFailures,
	}

	data := events.Event{
		EventID:             fmt.Sprintf("failure-%d", time.Now().UnixNano()),
		EventType:           "FAILURE_DETECTED",
		Severity:            "CRITICAL",
		Timestamp:           time.Now(),
		ServiceName:         "rate-limiter",
		InstanceID:          fd.InstanceID,
		MachineID:           fd.MachineID,
		Status:              string(Failed),
		FailureType:         string(reason),
		ConsecutiveFailures: consecutiveFailures,
		Message:             "Primary service failure detected",
	}

	// Send monitoring event
	if fd.EventReporter != nil {
		err := fd.EventReporter.Report(
			context.Background(),
			data,
		)

		if err != nil {
			fmt.Println("Failed to report failure event:", err)
		} else {
			fmt.Println("Failure event reported to monitoring backend")
		}
	}

	// Send failure notification to Recovery Manager
	fd.notifyRecoveryManager(event)
}

func (fd *FailureDetector) notifyRecoveryManager(event map[string]interface{}) {

	data, err := json.Marshal(event)

	if err != nil {
		fmt.Println("Failed to create failure event:", err)
		return
	}

	resp, err := fd.recoveryClient.Post(
		fd.RecoveryURL,
		"application/json",
		bytes.NewBuffer(data),
	)

	if err != nil {
		fmt.Println("Failed to notify Recovery Manager:", err)
		return
	}

	defer resp.Body.Close()

	fmt.Println(
		"Recovery Manager notified:",
		resp.StatusCode,
	)
}

func (fd *FailureDetector) sendHealthyEvent(responseTimeMS int64) {

	if fd.EventReporter == nil {
		return
	}

	event := events.Event{
		EventID:        fmt.Sprintf("health-%d", time.Now().UnixNano()),
		EventType:      "HEALTH_STATUS",
		Severity:       "INFO",
		Timestamp:      time.Now(),
		ServiceName:    "rate-limiter",
		InstanceID:     fd.InstanceID,
		MachineID:      fd.MachineID,
		Status:         string(Healthy),
		Role:           "PRIMARY",
		ResponseTimeMS: responseTimeMS,
		Message:        "Primary service is healthy",
	}

	err := fd.EventReporter.Report(
		context.Background(),
		event,
	)

	if err != nil {
		fmt.Println("Failed to report health event:", err)
		return
	}

	fmt.Println("Health event reported to monitoring backend")
}

func (fd *FailureDetector) Start() {

	fmt.Println("Failure Detector started")
	fmt.Println("Target:", fd.TargetURL)

	ticker := time.NewTicker(fd.CheckInterval)
	defer ticker.Stop()

	consecutiveFailures := 0
	state := Healthy

	// Check the initial health and report the measured response time.
	healthy, reason, responseTimeMS := fd.checkHealth()

	if healthy {
		fmt.Println("Primary is HEALTHY")
		fd.sendHealthyEvent(responseTimeMS)
	} else {
		consecutiveFailures = 1
		state = Suspected

		fmt.Printf(
			"Initial health check failed (1/%d) → %s\n",
			fd.FailureThreshold,
			reason,
		)
	}

	for {
		<-ticker.C

		healthy, reason, responseTimeMS := fd.checkHealth()

		if healthy {
			consecutiveFailures = 0

			if state != Healthy {
				state = Healthy

				fmt.Println("Primary recovered → HEALTHY")

				// Report recovery with the measured response time.
				fd.sendHealthyEvent(responseTimeMS)
			}

			continue
		}

		consecutiveFailures++

		if consecutiveFailures < fd.FailureThreshold {
			state = Suspected

			fmt.Printf(
				"Primary health check failed (%d/%d) → %s\n",
				consecutiveFailures,
				fd.FailureThreshold,
				reason,
			)

			continue
		}

		if state != Failed {
			state = Failed

			fmt.Printf(
				"Primary FAILED after %d consecutive failures → %s\n",
				consecutiveFailures,
				reason,
			)

			fd.sendFailureEvent(
				reason,
				consecutiveFailures,
			)
		}
	}
}
