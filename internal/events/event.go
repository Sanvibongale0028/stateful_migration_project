package events

import "time"

type Event struct {
	EventID   string    `json:"event_id"`
	EventType string    `json:"event_type"`
	Severity  string    `json:"severity"`
	Timestamp time.Time `json:"timestamp"`

	ServiceName string `json:"service_name"`
	InstanceID  string `json:"instance_id"`
	MachineID   string `json:"machine_id,omitempty"`

	Status string `json:"status,omitempty"`
	Role   string `json:"role,omitempty"`

	FailureType         string `json:"failure_type,omitempty"`
	ConsecutiveFailures int    `json:"consecutive_failures,omitempty"`

	FailedInstance   string `json:"failed_instance,omitempty"`
	PromotedInstance string `json:"promoted_instance,omitempty"`

	RecoveryStartedAt   *time.Time `json:"recovery_started_at,omitempty"`
	RecoveryCompletedAt *time.Time `json:"recovery_completed_at,omitempty"`

	// DetectionTimeMS int64 `json:"detection_time_ms,omitempty"`
	ResponseTimeMS  int64 `json:"response_time_ms,omitempty"`
	DetectionTimeMS int64 `json:"detection_time_ms,omitempty"`
	RecoveryTimeMS  int64 `json:"recovery_time_ms,omitempty"`
	DowntimeMS      int64 `json:"downtime_ms,omitempty"`

	StatePreserved bool `json:"state_preserved,omitempty"`

	Message string `json:"message,omitempty"`
}
