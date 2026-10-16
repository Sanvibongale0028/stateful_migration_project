package main

import (
	"os"

	"ecommerce-rate-limiter/internal/recovery"
)

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func main() {
	rm := recovery.NewRecoveryManager(
		getEnv("RECOVERY_PORT", "5000"),
		getEnv("SHADOW_URL", "http://localhost:4001"),
	)

	if err := rm.Start(); err != nil {
		panic(err)
	}
}

// package main

// import "ecommerce-rate-limiter/internal/recovery"

// func main() {

// 	rm := recovery.NewRecoveryManager(
// 		"5000",
// 		"http://localhost:4001",
// 	)

// 	if err := rm.Start(); err != nil {
// 		panic(err)
// 	}
// }