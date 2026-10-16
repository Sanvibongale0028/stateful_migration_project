package main

import (
	"os"
	"strconv"
	"time"

	"ecommerce-rate-limiter/internal/detector"
	"ecommerce-rate-limiter/internal/events"
)

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}

	return duration
}

func getInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	number, err := strconv.Atoi(value)
	if err != nil || number <= 0 {
		return fallback
	}

	return number
}

func main() {
	eventReporter := events.NewReporter(
		getEnv("EVENT_REPORTING_URL", "http://localhost:8090/events"),
	)

	fd := detector.NewFailureDetector(
		getEnv("TARGET_URL", "http://localhost:4000/health"),
		getEnv("RECOVERY_URL", "http://localhost:5000/failure"),
		getDuration("CHECK_INTERVAL", 2*time.Second),
		getDuration("REQUEST_TIMEOUT", time.Second),
		getInt("FAILURE_THRESHOLD", 3),
		getEnv("RATE_LIMITER_INSTANCE_ID", "rate-limiter-primary"),
		getEnv("MACHINE_ID", "machine-01"),
		eventReporter,
	)

	fd.Start()
}

// package main

// import (
// 	"time"

// 	"ecommerce-rate-limiter/internal/detector"
// 	"ecommerce-rate-limiter/internal/events"
// )

// func main() {

// 	eventReporter := events.NewReporter(
// 		"http://localhost:8090/events",
// 	)

// 	fd := detector.NewFailureDetector(
// 		// "http://localhost:4000/slow-health",
// 		"http://localhost:4000/health",
// 		"http://localhost:5000/failure",
// 		2*time.Second,
// 		1*time.Second,
// 		3,
// 		"primary-local",
// 		"machine-01",
// 		eventReporter,
// 	)

// 	fd.Start()
// }

