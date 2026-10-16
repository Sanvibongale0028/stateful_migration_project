package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type Event struct {
	EventID             string     `json:"event_id"`
	EventType           string     `json:"event_type"`
	Severity            string     `json:"severity"`
	Timestamp           time.Time  `json:"timestamp"`
	ServiceName         string     `json:"service_name"`
	InstanceID          string     `json:"instance_id"`
	MachineID           string     `json:"machine_id,omitempty"`
	Status              string     `json:"status,omitempty"`
	Role                string     `json:"role,omitempty"`
	FailureType         string     `json:"failure_type,omitempty"`
	ConsecutiveFailures int        `json:"consecutive_failures,omitempty"`
	FailedInstance      string     `json:"failed_instance,omitempty"`
	PromotedInstance    string     `json:"promoted_instance,omitempty"`
	RecoveryStartedAt   *time.Time `json:"recovery_started_at,omitempty"`
	RecoveryCompletedAt *time.Time `json:"recovery_completed_at,omitempty"`
	DetectionTimeMS     int64      `json:"detection_time_ms,omitempty"`
	ResponseTimeMS      int64      `json:"response_time_ms,omitempty"`
	RecoveryTimeMS      int64      `json:"recovery_time_ms,omitempty"`
	DowntimeMS          int64      `json:"downtime_ms,omitempty"`
	StatePreserved      bool       `json:"state_preserved,omitempty"`
	Message             string     `json:"message,omitempty"`
}

func eventsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	defer r.Body.Close()

	var event Event

	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	log.Println("========================================")
	log.Println("EVENT RECEIVED")
	log.Println("========================================")

	data, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		http.Error(w, "failed to format event", http.StatusInternalServerError)
		return
	}

	log.Println(string(data))
	log.Println("========================================")

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(map[string]string{
		"status": "received",
	})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/events", eventsHandler)
	mux.HandleFunc("/health", healthHandler)

	server := &http.Server{
		Addr:    ":8090",
		Handler: mux,
	}

	log.Println("Event receiver running on :8090")
	log.Println("POST /events")
	log.Println("GET  /health")

	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
