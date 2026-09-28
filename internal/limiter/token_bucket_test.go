package limiter

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestTokenBucket(t *testing.T) {

	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	ctx := context.Background()

	// Clean test key
	key := "rate_limit:test_client:/test"

	client.Del(ctx, key)

	bucket := NewTokenBucket(
		client,
		5,
		1,
	)

	// First request should be allowed
	result, err := bucket.Allow(ctx, key)

	if err != nil {
		t.Fatal(err)
	}

	if !result.Allowed {
		t.Fatal("expected first request to be allowed")
	}

	if result.Remaining >= 5 {
		t.Fatalf(
			"expected remaining tokens < 5, got %f",
			result.Remaining,
		)
	}

	// Cleanup
	client.Del(ctx, key)
	client.Close()
}

func TestTokenBucketRejectsAfterCapacity(t *testing.T) {

	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	ctx := context.Background()

	key := "rate_limit:test_rejection:/test"

	client.Del(ctx, key)

	bucket := NewTokenBucket(
		client,
		5,
		1,
	)

	// Consume all 5 tokens
	for i := 0; i < 5; i++ {

		result, err := bucket.Allow(ctx, key)

		if err != nil {
			t.Fatal(err)
		}

		if !result.Allowed {
			t.Fatalf(
				"request %d should be allowed",
				i+1,
			)
		}
	}

	// 6th request should be rejected
	result, err := bucket.Allow(ctx, key)

	if err != nil {
		t.Fatal(err)
	}

	if result.Allowed {
		t.Fatal("expected 6th request to be rejected")
	}

	client.Del(ctx, key)
	client.Close()
}
