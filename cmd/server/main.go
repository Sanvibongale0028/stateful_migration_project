package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"ecommerce-rate-limiter/config"
	"ecommerce-rate-limiter/internal/handler"
	"ecommerce-rate-limiter/internal/limiter"
	"ecommerce-rate-limiter/internal/logging"
	"ecommerce-rate-limiter/internal/store"
)

// --------------------------------------------------
// Primary Lease Renewal
// --------------------------------------------------
//
// Exactly ONE renewal goroutine is created for the
// lifetime of this server process.
//
// The goroutine checks the Handler's current role:
//
//	primary -> renew Redis lease
//	shadow  -> do NOT renew
//
// This is important because promotion/demotion changes
// the role without restarting the process.
//

func startLeaseRenewal(
	redisStore *store.RedisStore,
	instanceID string,
	isPrimary func() bool,
	setShadow func(),
	once *sync.Once,
) {
	once.Do(func() {

		ticker := time.NewTicker(2 * time.Second)

		go func() {
			defer ticker.Stop()

			for range ticker.C {

				// --------------------------------------------------
				// Check current role.
				// --------------------------------------------------
				//
				// If the instance has been demoted to shadow,
				// do not renew the primary lease.
				//

				if !isPrimary() {
					continue
				}

				ctx := context.Background()

				renewed, err := redisStore.RenewPrimary(
					ctx,
					instanceID,
				)

				// if err != nil {
				// 	log.Printf(
				// 		"Primary lease renewal failed: %v",
				// 		err,
				// 	)
				// 	continue
				// }
				if err != nil {
					_ = logging.Log(
						"FAILOVER",
						instanceID,
						"service_recovered",
						"WARN",
						"primary lease renewal failed; switching to shadow",
						map[string]string{
							"layer":  "RATE-LIMITER",
							"reason": "lease_renewal_error",
							"error":  err.Error(),
						},
					)

					setShadow()
					continue
				}

				if !renewed {
					_ = logging.Log(
						"FAILOVER",
						instanceID,
						"service_recovered",
						"WARN",
						"primary lease ownership lost; switching to shadow",
						map[string]string{
							"layer":  "RATE-LIMITER",
							"reason": "lease_rejected",
						},
					)

					setShadow()
					continue
				}

				// if !renewed {
				// 	log.Printf(
				// 		"Primary lease renewal rejected: instance %s no longer owns primary",
				// 		instanceID,
				// 	)
				// 	continue
				// }

				// log.Printf(
				// 	"Primary lease renewed by %s",
				// 	instanceID,
				// )
			}
		}()
	})
}

func main() {

	cfg := config.Load()

	// --------------------------------------------------
	// Backend reverse proxy
	// --------------------------------------------------

	backendURL, err := url.Parse(cfg.BackendURL)
	if err != nil {
		log.Fatalf(
			"invalid backend URL: %v",
			err,
		)
	}

	backendProxy := httputil.NewSingleHostReverseProxy(
		backendURL,
	)

	// Temporary proxy route for testing.
	proxyHandler := http.StripPrefix(
		"/proxy",
		backendProxy,
	)

	http.Handle(
		"/proxy/",
		proxyHandler,
	)

	// --------------------------------------------------
	// Redis
	// --------------------------------------------------

	redisStore := store.NewRedisStore(
		cfg.RedisHost,
		cfg.RedisPort,
	)

	ctx := context.Background()

	// Check Redis connection first.
	if err := redisStore.Ping(ctx); err != nil {
		log.Fatalf(
			"failed to connect to Redis: %v",
			err,
		)
	}

	_ = logging.Log(
		"RATE-LIMITER",
		cfg.InstanceID,
		"service_recovered",
		"INFO",
		"Redis connection established",
		nil,
	)

	// --------------------------------------------------
	// Primary Ownership
	// --------------------------------------------------
	//
	// If this process is configured as primary, it must
	// acquire the distributed Redis lease BEFORE starting.
	//
	// We intentionally do NOT start lease renewal here
	// because the Handler has not been created yet.
	//

	if cfg.Role == "primary" {

		acquired, err := redisStore.AcquirePrimary(
			ctx,
			cfg.InstanceID,
		)

		if err != nil {
			log.Fatal(
				"Failed to acquire primary ownership:",
				err,
			)
		}

		if !acquired {
			// currentPrimary, err := redisStore.GetPrimary(ctx)

			// if err != nil {
			// 	log.Fatal(
			// 		"Failed to determine current primary:",
			// 		err,
			// 	)
			// }

			// _ = logging.Log(
			// 	"FAILOVER",
			// 	cfg.InstanceID,
			// 	"service_recovered",
			// 	"INFO",
			// 	"starting as shadow",
			// 	map[string]string{
			// 		"current_primary": currentPrimary,
			// 		"reason":          "primary_lease_already_owned",
			// 		"layer":           "RATE-LIMITER",
			// 	},
			// )
			cfg.Role = "shadow"
		}

		// if !acquired {

		// 	currentPrimary, err := redisStore.GetPrimary(
		// 		ctx,
		// 	)

		// 	if err != nil {
		// 		log.Fatal(
		// 			"Failed to determine current primary:",
		// 			err,
		// 		)
		// 	}

		// 	log.Fatalf(
		// 		"Cannot start as primary. Current primary: %s",
		// 		currentPrimary,
		// 	)
		// }

		fmt.Println(
			"Primary ownership acquired by:",
			cfg.InstanceID,
		)
	}

	// --------------------------------------------------
	// Token Bucket
	// --------------------------------------------------

	tokenBucket := limiter.NewTokenBucket(
		redisStore.Client,
		cfg.Capacity,
		cfg.RefillRate,
	)

	// --------------------------------------------------
	// Handler
	// --------------------------------------------------
	//
	// h must exist before the lease-renewal callback can
	// access h.RoleMutex and h.ServiceRole.
	//

	var h *handler.Handler

	// --------------------------------------------------
	// Lease Renewal Control
	// --------------------------------------------------
	//
	// sync.Once guarantees that only ONE renewal goroutine
	// exists for this server process.
	//
	// This is important because /promote can be called
	// multiple times.
	//

	var leaseOnce sync.Once

	startRenewal := func() {

		startLeaseRenewal(
			redisStore,
			cfg.InstanceID,

			func() bool {

				return h.IsPrimary()
				// h.RoleMutex.RLock()
				// defer h.RoleMutex.RUnlock()

				// return h.ServiceRole == "primary"
			},

			func() {
				h.SetRole("shadow")
			},

			&leaseOnce,
		)
	}

	// --------------------------------------------------
	// Create Handler
	// --------------------------------------------------

	h = handler.NewHandler(
		tokenBucket,
		redisStore,
		backendProxy,
		cfg.Role,
		cfg.InstanceID,
		startRenewal,
	)

	// --------------------------------------------------
	// Start ONE Lease Renewal Goroutine
	// --------------------------------------------------
	//
	// This is intentionally called AFTER h is initialized.
	//
	// For Primary:
	//
	//     role = primary
	//     -> renewal starts
	//
	// For Shadow:
	//
	//     role = shadow
	//     -> goroutine exists but does nothing
	//
	// If Shadow is later promoted:
	//
	//     role = primary
	//     -> same goroutine starts renewing
	//
	// If Primary is later demoted:
	//
	//     role = shadow
	//     -> same goroutine stops renewing
	//

	startRenewal()

	// --------------------------------------------------
	// Routes
	// --------------------------------------------------

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

	// --------------------------------------------------
	// Real Rate Limiter + Backend Proxy
	// --------------------------------------------------

	// http.Handle(
	// 	"/api/",
	// 	http.StripPrefix(
	// 		"/api",
	// 		http.HandlerFunc(h.Proxy),
	// 	),
	// )

	http.Handle(
		"/api/",
		http.HandlerFunc(h.Proxy),
	)

	// --------------------------------------------------
	// Temporary Endpoint for Unresponsive Failure Testing
	// --------------------------------------------------

	http.HandleFunc(
		"/slow-health",
		h.SlowHealth,
	)

	// --------------------------------------------------
	// Prometheus Metrics
	// --------------------------------------------------

	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {

		role := h.GetRole()

		active := 0

		if role == "primary" {
			active = 1
		}

		w.Header().Set(
			"Content-Type",
			"text/plain; version=0.0.4; charset=utf-8",
		)

		fmt.Fprintln(w, "# HELP rate_limiter_active Whether this rate limiter instance is active.")
		fmt.Fprintln(w, "# TYPE rate_limiter_active gauge")

		fmt.Fprintf(
			w,
			"rate_limiter_active{instance=%q,role=%q} %d\n",
			cfg.InstanceID,
			role,
			active,
		)
	})
	// --------------------------------------------------
	// Server Information
	// --------------------------------------------------

	log.Printf(
		"Rate limiter running on :%s",
		cfg.Port,
	)

	log.Printf(
		"Role: %s",
		cfg.Role,
	)

	log.Printf(
		"Instance ID: %s",
		cfg.InstanceID,
	)

	log.Printf(
		"Capacity: %.2f",
		cfg.Capacity,
	)

	log.Printf(
		"Refill rate: %.2f tokens/second",
		cfg.RefillRate,
	)

	// --------------------------------------------------
	// Start Server
	// --------------------------------------------------

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
