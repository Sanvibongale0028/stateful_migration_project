package logging

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

var requestCounter uint64

func GetRequestID(r *http.Request) string {
	if id := r.Header.Get("X-Request-ID"); id != "" {
		return id
	}

	counter := atomic.AddUint64(&requestCounter, 1)

	return fmt.Sprintf(
		"r-%d-%d",
		time.Now().UnixMilli(),
		counter,
	)
}
