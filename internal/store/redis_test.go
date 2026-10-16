package store

import (
	"context"
	"testing"
	"time"
)

func TestPrimaryLeaseRenewal(t *testing.T) {

	ctx := context.Background()

	store := NewRedisStore(
		"localhost",
		"6379",
	)

	// Clean previous ownership.
	err := store.Client.Del(
		ctx,
		"rate_limiter:primary",
	).Err()

	if err != nil {
		t.Fatal(err)
	}

	instanceID := "test-primary"

	// Acquire the lease.
	acquired, err := store.AcquirePrimary(
		ctx,
		instanceID,
	)

	if err != nil {
		t.Fatal(err)
	}

	if !acquired {
		t.Fatal("failed to acquire primary lease")
	}

	// Verify ownership.
	owner, err := store.GetPrimary(ctx)

	if err != nil {
		t.Fatal(err)
	}

	if owner != instanceID {
		t.Fatalf(
			"expected owner %s, got %s",
			instanceID,
			owner,
		)
	}

	// Wait a little so TTL decreases.
	time.Sleep(2 * time.Second)

	ttlBefore, err := store.Client.TTL(
		ctx,
		"rate_limiter:primary",
	).Result()

	if err != nil {
		t.Fatal(err)
	}

	// Renew the lease.
	renewed, err := store.RenewPrimary(
		ctx,
		instanceID,
	)

	if err != nil {
		t.Fatal(err)
	}

	if !renewed {
		t.Fatal("failed to renew primary lease")
	}

	// Check TTL after renewal.
	ttlAfter, err := store.Client.TTL(
		ctx,
		"rate_limiter:primary",
	).Result()

	if err != nil {
		t.Fatal(err)
	}

	if ttlAfter <= ttlBefore {
		t.Fatalf(
			"expected TTL to increase: before=%v after=%v",
			ttlBefore,
			ttlAfter,
		)
	}

	if ttlAfter > 10*time.Second {
		t.Fatalf(
			"TTL should not exceed 10 seconds: %v",
			ttlAfter,
		)
	}

	// Cleanup.
	store.Client.Del(
		ctx,
		"rate_limiter:primary",
	)
}