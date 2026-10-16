package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port       string
	RedisHost  string
	RedisPort  string
	Capacity   float64
	RefillRate float64
	Role       string
	InstanceID string
	BackendURL string

	EventReportingURL string
	MachineID         string
}

func Load() Config {
	return Config{
		Port: getEnv("RATE_LIMITER_PORT", "4000"),

		RedisHost: getEnv("REDIS_HOST", "localhost"),

		RedisPort: getEnv("REDIS_PORT", "6379"),

		Capacity: getFloat(
			"RATE_LIMIT_CAPACITY",
			5,
		),

		RefillRate: getFloat(
			"RATE_LIMIT_REFILL_RATE",
			1,
		),

		Role: getEnv(
			"RATE_LIMITER_ROLE",
			"primary",
		),

		InstanceID: getEnv(
			"RATE_LIMITER_INSTANCE_ID",
			"rate-limiter-unknown",
		),

		BackendURL: getEnv("BACKEND_URL", "http://localhost:8080"),

		EventReportingURL: getEnv(
			"EVENT_REPORTING_URL",
			"http://localhost:8080/events",
		),

		MachineID: getEnv(
			"MACHINE_ID",
			"machine-01",
		),
	}
}

func getEnv(key string, defaultValue string) string {
	value := os.Getenv(key)

	if value == "" {
		return defaultValue
	}

	return value
}

func getFloat(key string, defaultValue float64) float64 {
	value := os.Getenv(key)

	if value == "" {
		return defaultValue
	}

	parsed, err := strconv.ParseFloat(value, 64)

	if err != nil {
		return defaultValue
	}

	return parsed
}
