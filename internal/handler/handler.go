package handler

import (
	"encoding/json"
	"net/http"
	"sync"

	"ecommerce-rate-limiter/internal/limiter"
)

type Handler struct {
	Limiter     *limiter.TokenBucket
	ServiceRole string
	RoleMutex   sync.RWMutex
}

func NewHandler(
	tokenBucket *limiter.TokenBucket,
	role string,
) *Handler {

	return &Handler{
		Limiter:     tokenBucket,
		ServiceRole: role,
	}
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
	resource := "/api/test"

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
	h.SetRole("primary")

	role := h.GetRole()

	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	json.NewEncoder(w).Encode(
		map[string]interface{}{
			"status": "promoted",
			"role":   role,
			"active": role == "primary",
		},
	)
}

func (h *Handler) Demote(
	w http.ResponseWriter,
	r *http.Request,
) {
	h.SetRole("shadow")

	role := h.GetRole()

	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	json.NewEncoder(w).Encode(
		map[string]interface{}{
			"status": "demoted",
			"role":   role,
			"active": role == "primary",
		},
	)
}
