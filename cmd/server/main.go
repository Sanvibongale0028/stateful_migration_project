package main

import (
	"context"
	"log"
	"net/http"

	"ecommerce-rate-limiter/config"
	"ecommerce-rate-limiter/internal/handler"
	"ecommerce-rate-limiter/internal/limiter"
	"ecommerce-rate-limiter/internal/store"
)

func main() {

	cfg := config.Load()

	redisStore := store.NewRedisStore(
		cfg.RedisHost,
		cfg.RedisPort,
	)

	ctx := context.Background()

	if err := redisStore.Ping(ctx); err != nil {

		log.Fatalf(
			"failed to connect to Redis: %v",
			err,
		)
	}

	log.Println("Connected to Redis")

	tokenBucket := limiter.NewTokenBucket(
		redisStore.Client,
		cfg.Capacity,
		cfg.RefillRate,
	)

	h := handler.NewHandler(
		tokenBucket,
		cfg.Role,
	)

	http.HandleFunc(
		"/health",
		h.Health,
	)

	http.HandleFunc(
		"/role",
		h.Role,
	)

	http.HandleFunc(
		"/promote",
		h.Promote,
	)

	http.HandleFunc(
		"/demote",
		h.Demote,
	)

	http.HandleFunc(
		"/check",
		h.Check,
	)

	log.Printf(
		"Rate limiter running on :%s",
		cfg.Port,
	)

	log.Printf(
		"Role: %s",
		cfg.Role,
	)

	log.Printf(
		"Capacity: %.2f",
		cfg.Capacity,
	)

	log.Printf(
		"Refill rate: %.2f tokens/second",
		cfg.RefillRate,
	)

	if err := http.ListenAndServe(
		":"+cfg.Port,
		nil,
	); err != nil {

		log.Fatalf(
			"server stopped: %v",
			err,
		)
	}
}
