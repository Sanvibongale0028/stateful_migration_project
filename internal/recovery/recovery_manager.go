package recovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"ecommerce-rate-limiter/internal/events"
)

type FailureEvent struct {
	Service             string    `json:"service"`
	FailureType         string    `json:"failure_type"`
	DetectedAt          time.Time `json:"detected_at"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
}

type RecoveryManager struct {
	Port         string
	SecondaryURL string
	Client       *http.Client

	EventReporter *events.Reporter

	InstanceID        string
	PrimaryInstanceID string
	MachineID         string
}

// type RecoveryManager struct {
// 	Port         string
// 	SecondaryURL string
// 	Client       *http.Client

// 	EventReporter *events.Reporter
// }

// func NewRecoveryManager(
// 	port string,
// 	secondaryURL string,
// ) *RecoveryManager {

// 	return &RecoveryManager{
// 		Port:         port,
// 		SecondaryURL: secondaryURL,

// 		Client: &http.Client{
// 			Timeout: 2 * time.Second,
// 		},

// 		EventReporter: events.NewReporter(
// 			"http://localhost:8090/events",
// 		),
// 	}
// }

func NewRecoveryManager(
	port string,
	secondaryURL string,
) *RecoveryManager {

	getEnv := func(key, fallback string) string {
		if value := os.Getenv(key); value != "" {
			return value
		}
		return fallback
	}

	return &RecoveryManager{
		Port:         port,
		SecondaryURL: secondaryURL,

		Client: &http.Client{
			Timeout: 2 * time.Second,
		},

		EventReporter: events.NewReporter(
			getEnv(
				"EVENT_REPORTING_URL",
				"http://localhost:8090/events",
			),
		),

		InstanceID:        getEnv("RATE_LIMITER_INSTANCE_ID", "rate-limiter-secondary"),
		PrimaryInstanceID: getEnv("PRIMARY_INSTANCE_ID", "rate-limiter-primary"),
		MachineID:         getEnv("MACHINE_ID", "machine-01"),
	}
}

// reportEvent sends a structured event to the event receiver.
func (rm *RecoveryManager) reportEvent(event events.Event) {
	if rm.EventReporter == nil {
		return
	}

	if err := rm.EventReporter.Report(
		context.Background(),
		event,
	); err != nil {
		fmt.Println("Failed to report recovery event:", err)
		return
	}

	fmt.Println("Recovery event reported:", event.EventType)
}

// HandleFailure receives a failure notification from the Failure Detector.
func (rm *RecoveryManager) HandleFailure(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodPost {
		http.Error(
			w,
			"method not allowed",
			http.StatusMethodNotAllowed,
		)
		return
	}

	var failure FailureEvent

	if err := json.NewDecoder(r.Body).Decode(&failure); err != nil {
		http.Error(
			w,
			"invalid failure event",
			http.StatusBadRequest,
		)
		return
	}
	defer r.Body.Close()

	fmt.Println("Failure event received")
	fmt.Println("Service:", failure.Service)
	fmt.Println("Failure type:", failure.FailureType)
	fmt.Println("Consecutive failures:", failure.ConsecutiveFailures)
	fmt.Println("Detected at:", failure.DetectedAt)

	recoveryStartedAt := time.Now()

	// Report that recovery has started.
	rm.reportEvent(events.Event{
		EventID:           fmt.Sprintf("recovery-started-%d", time.Now().UnixNano()),
		EventType:         "RECOVERY_STARTED",
		Severity:          "WARNING",
		Timestamp:         recoveryStartedAt,
		ServiceName:       failure.Service,
		InstanceID:        rm.InstanceID,
		MachineID:         rm.MachineID,
		Status:            "RECOVERING",
		FailedInstance:    rm.PrimaryInstanceID,
		RecoveryStartedAt: &recoveryStartedAt,
		Message:           "Recovery Manager started failover",
	})

	err := rm.PromoteSecondary()

	if err != nil {
		fmt.Println("Failed to promote secondary:", err)

		now := time.Now()

		rm.reportEvent(events.Event{
			EventID:           fmt.Sprintf("recovery-failed-%d", now.UnixNano()),
			EventType:         "RECOVERY_FAILED",
			Severity:          "CRITICAL",
			Timestamp:         now,
			ServiceName:       failure.Service,
			InstanceID:        rm.InstanceID,
			MachineID:         rm.MachineID,
			Status:            "FAILED",
			FailedInstance:    rm.PrimaryInstanceID,
			RecoveryStartedAt: &recoveryStartedAt,
			RecoveryTimeMS:    time.Since(recoveryStartedAt).Milliseconds(),
			Message:           err.Error(),
		})

		http.Error(
			w,
			"recovery failed",
			http.StatusInternalServerError,
		)
		return
	}

	recoveryCompletedAt := time.Now()
	recoveryTimeMS := recoveryCompletedAt.Sub(
		recoveryStartedAt,
	).Milliseconds()

	// Report verified promotion.
	rm.reportEvent(events.Event{
		EventID:             fmt.Sprintf("secondary-promoted-%d", time.Now().UnixNano()),
		EventType:           "SECONDARY_PROMOTED",
		Severity:            "INFO",
		Timestamp:           recoveryCompletedAt,
		ServiceName:         failure.Service,
		InstanceID:          rm.InstanceID,
		MachineID:           rm.MachineID,
		Status:              "ACTIVE",
		Role:                "PRIMARY",
		FailedInstance:      rm.PrimaryInstanceID,
		PromotedInstance:    rm.InstanceID,
		RecoveryStartedAt:   &recoveryStartedAt,
		RecoveryCompletedAt: &recoveryCompletedAt,
		RecoveryTimeMS:      recoveryTimeMS,
		Message:             "Secondary promoted and primary role verified",
	})

	// Report completion of the failover procedure.
	rm.reportEvent(events.Event{
		EventID:             fmt.Sprintf("recovery-completed-%d", time.Now().UnixNano()),
		EventType:           "RECOVERY_COMPLETED",
		Severity:            "INFO",
		Timestamp:           recoveryCompletedAt,
		ServiceName:         failure.Service,
		InstanceID:          rm.InstanceID,
		MachineID:           rm.MachineID,
		Status:              "COMPLETED",
		Role:                "PRIMARY",
		FailedInstance:      rm.PrimaryInstanceID,
		PromotedInstance:    rm.InstanceID,
		RecoveryStartedAt:   &recoveryStartedAt,
		RecoveryCompletedAt: &recoveryCompletedAt,
		RecoveryTimeMS:      recoveryTimeMS,
		Message:             "Recovery completed successfully",
	})

	w.Header().Set("Content-Type", "application/json")

	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "failover_completed",
	})
}

// PromoteSecondary requests promotion and verifies the resulting role.
func (rm *RecoveryManager) PromoteSecondary() error {
	maxAttempts := 5
	retryDelay := 2 * time.Second

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		fmt.Printf(
			"Attempting secondary promotion (%d/%d)\n",
			attempt,
			maxAttempts,
		)

		req, err := http.NewRequest(
			http.MethodPost,
			rm.SecondaryURL+"/promote",
			nil,
		)
		if err != nil {
			return err
		}

		resp, err := rm.Client.Do(req)
		if err != nil {
			return fmt.Errorf(
				"secondary promotion request failed: %w",
				err,
			)
		}

		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			fmt.Println("Secondary promotion request accepted")

			roleResp, err := rm.Client.Get(
				rm.SecondaryURL + "/role",
			)
			if err != nil {
				return fmt.Errorf(
					"failed to verify secondary role: %w",
					err,
				)
			}

			var roleData struct {
				Role   string `json:"role"`
				Active bool   `json:"active"`
			}

			decodeErr := json.NewDecoder(roleResp.Body).Decode(&roleData)
			roleResp.Body.Close()

			if decodeErr != nil {
				return fmt.Errorf(
					"failed to decode secondary role: %w",
					decodeErr,
				)
			}

			if roleResp.StatusCode != http.StatusOK {
				return fmt.Errorf(
					"secondary role verification failed: status %d",
					roleResp.StatusCode,
				)
			}

			if roleData.Role != "primary" || !roleData.Active {
				return fmt.Errorf(
					"secondary promotion verification failed: role=%s active=%t",
					roleData.Role,
					roleData.Active,
				)
			}

			fmt.Println("Secondary promotion verified successfully")
			return nil
		}

		if resp.StatusCode == http.StatusConflict {
			fmt.Printf(
				"Promotion blocked by existing primary lease (%d/%d)\n",
				attempt,
				maxAttempts,
			)

			if attempt < maxAttempts {
				time.Sleep(retryDelay)
				continue
			}

			return fmt.Errorf(
				"primary lease did not expire after %d attempts",
				maxAttempts,
			)
		}

		return fmt.Errorf(
			"secondary promotion request failed: status %d",
			resp.StatusCode,
		)
	}

	return fmt.Errorf(
		"secondary promotion failed after %d attempts",
		maxAttempts,
	)
}

func (rm *RecoveryManager) Start() error {
	mux := http.NewServeMux()

	mux.HandleFunc("/failure", rm.HandleFailure)

	fmt.Println("Recovery Manager started on port", rm.Port)

	return http.ListenAndServe(
		":"+rm.Port,
		mux,
	)
}


// package recovery

// import (
// 	"encoding/json"
// 	"fmt"
// 	"net/http"
// 	"time"
// )

// type FailureEvent struct {
// 	Service             string    `json:"service"`
// 	FailureType         string    `json:"failure_type"`
// 	DetectedAt          time.Time `json:"detected_at"`
// 	ConsecutiveFailures int       `json:"consecutive_failures"`
// }

// type RecoveryManager struct {
// 	Port         string
// 	SecondaryURL string
// 	Client       *http.Client
// }

// func NewRecoveryManager(port string,
// 	secondaryURL string,
// ) *RecoveryManager {
// 	return &RecoveryManager{
// 		Port:         port,
// 		SecondaryURL: secondaryURL,
// 		Client: &http.Client{
// 			Timeout: 2 * time.Second,
// 		},
// 	}
// }

// func (rm *RecoveryManager) HandleFailure(
// 	w http.ResponseWriter,
// 	r *http.Request,
// ) {

// 	if r.Method != http.MethodPost {
// 		http.Error(
// 			w,
// 			"method not allowed",
// 			http.StatusMethodNotAllowed,
// 		)
// 		return
// 	}

// 	var event FailureEvent

// 	err := json.NewDecoder(r.Body).Decode(&event)

// 	if err != nil {
// 		http.Error(
// 			w,
// 			"invalid failure event",
// 			http.StatusBadRequest,
// 		)
// 		return
// 	}

// 	fmt.Println("Failure event received")
// 	fmt.Println("Service:", event.Service)
// 	fmt.Println("Failure type:", event.FailureType)
// 	fmt.Println(
// 		"Consecutive failures:",
// 		event.ConsecutiveFailures,
// 	)
// 	fmt.Println("Detected at:", event.DetectedAt)

// 	err = rm.PromoteSecondary()

// 	if err != nil {

// 		fmt.Println(
// 			"Failed to promote secondary:",
// 			err,
// 		)

// 		http.Error(
// 			w,
// 			"recovery failed",
// 			http.StatusInternalServerError,
// 		)

// 		return
// 	}

// 	w.Header().Set(
// 		"Content-Type",
// 		"application/json",
// 	)

// 	json.NewEncoder(w).Encode(
// 		map[string]string{
// 			"status": "recovery_started",
// 		},
// 	)
// }

// // func (rm *RecoveryManager) PromoteSecondary() error {

// // 	resp, err := rm.Client.Post(
// // 		rm.SecondaryURL+"/promote",
// // 		"application/json",
// // 		nil,
// // 	)

// // 	if err != nil {
// // 		return fmt.Errorf(
// // 			"failed to contact secondary: %w",
// // 			err,
// // 		)
// // 	}

// // 	resp.Body.Close()

// // 	if resp.StatusCode != http.StatusOK {
// // 		return fmt.Errorf(
// // 			"secondary promotion request failed: status %d",
// // 			resp.StatusCode,
// // 		)
// // 	}

// // 	fmt.Println("Promotion request accepted")

// // 	// Verify the actual role of the secondary.
// // 	verifyResp, err := rm.Client.Get(
// // 		rm.SecondaryURL + "/role",
// // 	)

// // 	if err != nil {
// // 		return fmt.Errorf(
// // 			"failed to verify secondary role: %w",
// // 			err,
// // 		)
// // 	}

// // 	defer verifyResp.Body.Close()

// // 	if verifyResp.StatusCode != http.StatusOK {
// // 		return fmt.Errorf(
// // 			"role verification failed: status %d",
// // 			verifyResp.StatusCode,
// // 		)
// // 	}

// // 	var roleResponse struct {
// // 		Role   string `json:"role"`
// // 		Active bool   `json:"active"`
// // 	}

// // 	err = json.NewDecoder(
// // 		verifyResp.Body,
// // 	).Decode(&roleResponse)

// // 	if err != nil {
// // 		return fmt.Errorf(
// // 			"failed to decode role response: %w",
// // 			err,
// // 		)
// // 	}

// // 	if roleResponse.Role != "primary" ||
// // 		!roleResponse.Active {

// // 		return fmt.Errorf(
// // 			"secondary promotion verification failed: role=%s active=%t",
// // 			roleResponse.Role,
// // 			roleResponse.Active,
// // 		)
// // 	}

// // 	fmt.Println(
// // 		"Secondary promotion verified successfully",
// // 	)

// // 	return nil
// // }

// func (rm *RecoveryManager) PromoteSecondary() error {

// 	maxAttempts := 5
// 	retryDelay := 2 * time.Second

// 	for attempt := 1; attempt <= maxAttempts; attempt++ {

// 		fmt.Printf(
// 			"Attempting secondary promotion (%d/%d)\n",
// 			attempt,
// 			maxAttempts,
// 		)

// 		req, err := http.NewRequest(
// 			http.MethodPost,
// 			rm.SecondaryURL+"/promote",
// 			nil,
// 		)

// 		if err != nil {
// 			return err
// 		}

// 		resp, err := rm.Client.Do(req)

// 		if err != nil {
// 			return fmt.Errorf(
// 				"secondary promotion request failed: %w",
// 				err,
// 			)
// 		}

// 		resp.Body.Close()

// 		// Promotion succeeded.
// 		if resp.StatusCode == http.StatusOK {
// 			fmt.Println(
// 				"Secondary promotion request accepted",
// 			)

// 			// Verify the secondary role.
// 			roleResp, err := rm.Client.Get(
// 				rm.SecondaryURL + "/role",
// 			)

// 			if err != nil {
// 				return fmt.Errorf(
// 					"failed to verify secondary role: %w",
// 					err,
// 				)
// 			}

// 			defer roleResp.Body.Close()

// 			if roleResp.StatusCode != http.StatusOK {
// 				return fmt.Errorf(
// 					"secondary role verification failed: status %d",
// 					roleResp.StatusCode,
// 				)
// 			}

// 			var roleData struct {
// 				Role   string `json:"role"`
// 				Active bool   `json:"active"`
// 			}

// 			if err := json.NewDecoder(
// 				roleResp.Body,
// 			).Decode(&roleData); err != nil {
// 				return fmt.Errorf(
// 					"failed to decode secondary role: %w",
// 					err,
// 				)
// 			}

// 			if roleData.Role != "primary" ||
// 				!roleData.Active {

// 				return fmt.Errorf(
// 					"secondary promotion verification failed: role=%s active=%t",
// 					roleData.Role,
// 					roleData.Active,
// 				)
// 			}

// 			fmt.Println(
// 				"Secondary promotion verified successfully",
// 			)

// 			return nil
// 		}

// 		// The old Primary's Redis lease may still exist.
// 		if resp.StatusCode == http.StatusConflict {

// 			fmt.Printf(
// 				"Secondary promotion blocked by existing primary lease (%d/%d)\n",
// 				attempt,
// 				maxAttempts,
// 			)

// 			if attempt < maxAttempts {
// 				time.Sleep(retryDelay)
// 				continue
// 			}

// 			return fmt.Errorf(
// 				"secondary promotion failed: primary lease did not expire after %d attempts",
// 				maxAttempts,
// 			)
// 		}

// 		return fmt.Errorf(
// 			"secondary promotion request failed: status %d",
// 			resp.StatusCode,
// 		)
// 	}

// 	return fmt.Errorf(
// 		"secondary promotion failed after %d attempts",
// 		maxAttempts,
// 	)
// }

// func (rm *RecoveryManager) Start() error {

// 	http.HandleFunc(
// 		"/failure",
// 		rm.HandleFailure,
// 	)

// 	fmt.Println(
// 		"Recovery Manager started on port",
// 		rm.Port,
// 	)

// 	return http.ListenAndServe(
// 		":"+rm.Port,
// 		nil,
// 	)
// }
