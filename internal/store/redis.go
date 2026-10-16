package store

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	primaryLeaseKey = "rate_limiter:primary"
	primaryLeaseTTL = 10 * time.Second
)

// Atomically renews the lease only if this instance still owns it.
var renewPrimaryScript = redis.NewScript(`
local current = redis.call("GET", KEYS[1])

if not current then
	return 0
end

if current ~= ARGV[1] then
	return 0
end

redis.call("EXPIRE", KEYS[1], ARGV[2])

return 1
`)

// Atomically releases the lease only if this instance still owns it.
var releasePrimaryScript = redis.NewScript(`
local current = redis.call("GET", KEYS[1])

if not current then
	return 0
end

if current ~= ARGV[1] then
	return -1
end

redis.call("DEL", KEYS[1])

return 1
`)

type RedisStore struct {
	Client *redis.Client
}

func NewRedisStore(host string, port string) *RedisStore {
	client := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%s", host, port),
	})

	return &RedisStore{
		Client: client,
	}
}

func (r *RedisStore) Ping(ctx context.Context) error {
	return r.Client.Ping(ctx).Err()
}

// AcquirePrimary attempts to acquire the primary lease.
//
// SETNX + expiration is performed atomically by Redis.
func (r *RedisStore) AcquirePrimary(
	ctx context.Context,
	instanceID string,
) (bool, error) {

	return r.Client.SetNX(
		ctx,
		primaryLeaseKey,
		instanceID,
		primaryLeaseTTL,
	).Result()
}

func (r *RedisStore) GetPrimary(
	ctx context.Context,
) (string, error) {

	return r.Client.Get(
		ctx,
		primaryLeaseKey,
	).Result()
}

// RenewPrimary renews the lease only when instanceID is still
// the current owner.
//
// The owner check and EXPIRE happen inside one Redis Lua script,
// so another instance cannot acquire the lease between those operations.
func (r *RedisStore) RenewPrimary(
	ctx context.Context,
	instanceID string,
) (bool, error) {

	result, err := renewPrimaryScript.Run(
		ctx,
		r.Client,
		[]string{primaryLeaseKey},
		instanceID,
		int(primaryLeaseTTL/time.Second),
	).Int()

	if err != nil {
		return false, err
	}

	return result == 1, nil
}

// ReleasePrimary releases the lease only when instanceID is still
// the current owner.
//
// This prevents an old primary from deleting a newer primary's lease.
func (r *RedisStore) ReleasePrimary(
	ctx context.Context,
	instanceID string,
) error {

	result, err := releasePrimaryScript.Run(
		ctx,
		r.Client,
		[]string{primaryLeaseKey},
		instanceID,
	).Int()

	if err != nil {
		return err
	}

	switch result {
	case 1:
		// Lease successfully released.
		return nil

	case 0:
		// Lease does not exist anymore.
		return nil

	case -1:
		// Another instance owns the lease.
		return fmt.Errorf(
			"instance %s does not own primary role",
			instanceID,
		)

	default:
		return fmt.Errorf(
			"unexpected Redis release result: %d",
			result,
		)
	}
}

// package store

// import (
// 	"context"
// 	"fmt"
// 	"time"

// 	"github.com/redis/go-redis/v9"
// )

// type RedisStore struct {
// 	Client *redis.Client
// }

// func NewRedisStore(host string, port string) *RedisStore {

// 	client := redis.NewClient(
// 		&redis.Options{
// 			Addr: fmt.Sprintf("%s:%s", host, port),
// 		},
// 	)

// 	return &RedisStore{
// 		Client: client,
// 	}
// }

// func (r *RedisStore) Ping(ctx context.Context) error {
// 	return r.Client.Ping(ctx).Err()
// }

// func (r *RedisStore) AcquirePrimary(
// 	ctx context.Context,
// 	instanceID string,
// ) (bool, error) {

// 	return r.Client.SetNX(
// 		ctx,
// 		"rate_limiter:primary",
// 		instanceID,
// 		10*time.Second,
// 	).Result()
// }

// func (r *RedisStore) RenewPrimary(
// 	ctx context.Context,
// 	instanceID string,
// ) (bool, error) {

// 	currentOwner, err := r.Client.Get(
// 		ctx,
// 		"rate_limiter:primary",
// 	).Result()

// 	if err != nil {
// 		if err == redis.Nil {
// 			return false, nil
// 		}

// 		return false, err
// 	}

// 	if currentOwner != instanceID {
// 		return false, nil
// 	}

// 	err = r.Client.Expire(
// 		ctx,
// 		"rate_limiter:primary",
// 		10*time.Second,
// 	).Err()

// 	if err != nil {
// 		return false, err
// 	}

// 	return true, nil
// }

// func (r *RedisStore) GetPrimary(
// 	ctx context.Context,
// ) (string, error) {

// 	return r.Client.Get(
// 		ctx,
// 		"rate_limiter:primary",
// 	).Result()
// }

// func (r *RedisStore) ReleasePrimary(
// 	ctx context.Context,
// 	instanceID string,
// ) error {

// 	current, err := r.GetPrimary(ctx)

// 	if err != nil {
// 		if err == redis.Nil {
// 			return nil
// 		}
// 		return err
// 	}

// 	if current != instanceID {
// 		return fmt.Errorf(
// 			"instance %s does not own primary role",
// 			instanceID,
// 		)
// 	}

// 	return r.Client.Del(
// 		ctx,
// 		"rate_limiter:primary",
// 	).Err()
// }
