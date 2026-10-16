package events

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Reporter struct {
	Endpoint string
	Client   *http.Client
}

func NewReporter(endpoint string) *Reporter {
	return &Reporter{
		Endpoint: endpoint,
		Client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (r *Reporter) Report(ctx context.Context, event Event) error {
	if r.Endpoint == "" {
		return nil
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		r.Endpoint,
		bytes.NewBuffer(data),
	)
	if err != nil {
		return fmt.Errorf("failed to create event request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := r.Client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send event: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf(
			"event reporting failed with status: %s",
			resp.Status,
		)
	}

	return nil
}
