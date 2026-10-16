package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httputil"
	"sync"
	"time"

	"ecommerce-rate-limiter/internal/limiter"
	"ecommerce-rate-limiter/internal/store"
)

type Handler struct {
	BackendProxy      *httputil.ReverseProxy
	Limiter           *limiter.TokenBucket
	RedisStore        *store.RedisStore
	ServiceRole       string
	InstanceID        string
	StartLeaseRenewal func()
	RoleMutex         sync.RWMutex
}

func NewHandler(
	tokenBucket *limiter.TokenBucket,
	redisStore *store.RedisStore,
	backendProxy *httputil.ReverseProxy,
	role string,
	instanceID string,
	startLeaseRenewal func(),
) *Handler {

	return &Handler{
		Limiter:           tokenBucket,
		RedisStore:        redisStore,
		BackendProxy:      backendProxy,
		ServiceRole:       role,
		InstanceID:        instanceID,
		StartLeaseRenewal: startLeaseRenewal,
	}
}

func (h *Handler) IsPrimary() bool {
    h.RoleMutex.RLock()
    defer h.RoleMutex.RUnlock()

    return h.ServiceRole == "primary"
}

func (h *Handler) Health(
	w http.ResponseWriter,
	r *http.Request,
) {

	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	json.NewEncoder(w).Encode(
		map[string]string{
			"status": "ok",
		},
	)
}

func (h *Handler) Check(
	w http.ResponseWriter,
	r *http.Request,
) {
	role := h.GetRole()

	if role != "primary" {
		w.Header().Set(
			"Content-Type",
			"application/json",
		)

		w.WriteHeader(http.StatusServiceUnavailable)

		json.NewEncoder(w).Encode(
			map[string]string{
				"status": "standby",
			},
		)

		return
	}

	clientID := r.URL.Query().Get("client_id")

	if clientID == "" {

		http.Error(
			w,
			"client_id is required",
			http.StatusBadRequest,
		)

		return
	}

	// Temporary generic resource.
	resource := r.URL.Query().Get("resource")

	if resource == "" {
		http.Error(
			w,
			"resource is required",
			http.StatusBadRequest,
		)
		return
	}

	key := limiter.BuildKey(
		clientID,
		resource,
	)

	result, err := h.Limiter.Allow(
		r.Context(),
		key,
	)

	if err != nil {

		http.Error(
			w,
			"rate limiter error",
			http.StatusInternalServerError,
		)

		return
	}

	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	if !result.Allowed {

		w.Header().Set("Retry-After", "1")

		w.WriteHeader(
			http.StatusTooManyRequests,
		)

		json.NewEncoder(w).Encode(
			map[string]interface{}{
				"allowed":   false,
				"remaining": result.Remaining,
			},
		)

		return
	}

	json.NewEncoder(w).Encode(
		map[string]interface{}{
			"allowed":   true,
			"remaining": result.Remaining,
		},
	)
}

func (h *Handler) Role(
	w http.ResponseWriter,
	r *http.Request,
) {

	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	role := h.GetRole()

	json.NewEncoder(w).Encode(
		map[string]interface{}{
			"role":   role,
			"active": role == "primary",
		},
	)
}

func (h *Handler) SetRole(role string) {
	h.RoleMutex.Lock()
	defer h.RoleMutex.Unlock()

	h.ServiceRole = role
}

func (h *Handler) GetRole() string {
	h.RoleMutex.RLock()
	defer h.RoleMutex.RUnlock()

	return h.ServiceRole
}

func (h *Handler) Promote(
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

	h.RoleMutex.RLock()
	currentRole := h.ServiceRole
	h.RoleMutex.RUnlock()

	if currentRole == "primary" {
		w.Header().Set(
			"Content-Type",
			"application/json",
		)

		json.NewEncoder(w).Encode(
			map[string]interface{}{
				"status": "already_primary",
				"role":   "primary",
				"active": true,
			},
		)

		return
	}

	ctx := r.Context()

	acquired, err := h.RedisStore.AcquirePrimary(
		ctx,
		h.InstanceID,
	)

	if err != nil {
		http.Error(
			w,
			"failed to acquire primary ownership",
			http.StatusInternalServerError,
		)
		return
	}

	if !acquired {

		currentPrimary, err := h.RedisStore.GetPrimary(ctx)

		if err != nil {
			http.Error(
				w,
				"failed to determine current primary",
				http.StatusInternalServerError,
			)
			return
		}

		http.Error(
			w,
			"primary already exists: "+currentPrimary,
			http.StatusConflict,
		)

		return
	}

	h.SetRole("primary")

	if h.StartLeaseRenewal != nil {
		h.StartLeaseRenewal()
	}

	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	json.NewEncoder(w).Encode(
		map[string]interface{}{
			"status":   "promoted",
			"role":     "primary",
			"active":   true,
			"instance": h.InstanceID,
		},
	)
}

func (h *Handler) Demote(w http.ResponseWriter, r *http.Request) {
    h.RoleMutex.Lock()

    if h.ServiceRole == "shadow" {
        h.RoleMutex.Unlock()

        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(map[string]interface{}{
            "status": "already_shadow",
            "role":   "shadow",
            "active": false,
        })
        return
    }

    h.ServiceRole = "shadow"
    h.RoleMutex.Unlock()

    // Immediately release the Redis primary lease.
    ctx := context.Background()

    if err := h.RedisStore.ReleasePrimary(ctx, h.InstanceID); err != nil {
        log.Printf("Failed to release primary lease during demotion: %v", err)

        http.Error(
            w,
            `{"status":"demotion_failed","error":"failed to release primary lease"}`,
            http.StatusInternalServerError,
        )
        return
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)

    json.NewEncoder(w).Encode(map[string]interface{}{
        "status": "demoted",
        "role":   "shadow",
        "active": false,
    })
}

func (h *Handler) SlowHealth(
	w http.ResponseWriter,
	r *http.Request,
) {
	time.Sleep(3 * time.Second)

	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	json.NewEncoder(w).Encode(
		map[string]string{
			"status": "ok",
		},
	)
}

func (h *Handler) Proxy(
	w http.ResponseWriter,
	r *http.Request,
) {
	role := h.GetRole()

	if role != "primary" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)

		json.NewEncoder(w).Encode(
			map[string]string{
				"status": "standby",
			},
		)

		return
	}

	clientID := r.URL.Query().Get("client_id")

	if clientID == "" {
		http.Error(
			w,
			"client_id is required",
			http.StatusBadRequest,
		)
		return
	}

	// The actual backend resource is the request path.
	resource := r.URL.Path

	key := limiter.BuildKey(
		clientID,
		resource,
	)

	result, err := h.Limiter.Allow(
		r.Context(),
		key,
	)

	if err != nil {
		http.Error(
			w,
			"rate limiter error",
			http.StatusInternalServerError,
		)
		return
	}

	if !result.Allowed {
		w.Header().Set("Retry-After", "1")
		w.Header().Set("Content-Type", "application/json")

		w.WriteHeader(http.StatusTooManyRequests)

		json.NewEncoder(w).Encode(
			map[string]interface{}{
				"allowed":   false,
				"remaining": result.Remaining,
			},
		)

		return
	}

	// client_id is only used by the rate limiter.
	// Do not send it to the backend as part of the query.
	query := r.URL.Query()
	query.Del("client_id")
	r.URL.RawQuery = query.Encode()

	// Forward the original request to the backend.
	h.BackendProxy.ServeHTTP(w, r)
}
