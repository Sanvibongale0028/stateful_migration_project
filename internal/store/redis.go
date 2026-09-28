package store

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

type RedisStore struct {
	Client *redis.Client
}

func NewRedisStore(host string, port string) *RedisStore {

	client := redis.NewClient(
		&redis.Options{
			Addr: fmt.Sprintf("%s:%s", host, port),
		},
	)

	return &RedisStore{
		Client: client,
	}
}

func (r *RedisStore) Ping(ctx context.Context) error {
	return r.Client.Ping(ctx).Err()
}
