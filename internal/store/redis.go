package store

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisStore struct {
	client *redis.Client
}

func NewRedisStore(addr string) *RedisStore {
	rdb := redis.NewClient(&redis.Options{
		Addr:         addr,
		PoolSize:     50,
		MinIdleConns: 10,
	})
	return &RedisStore{client: rdb}
}

func (s *RedisStore) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

// SlidingWindowIncr increments the sliding window counter and returns current count.
// Uses a sorted set: key = "rl:{clientID}:{resource}", score = timestamp, member = unique ID
func (s *RedisStore) SlidingWindowIncr(ctx context.Context, clientID, resource string, windowSec int64, cost int64) (int64, error) {
	key := fmt.Sprintf("rl:%s:%s", clientID, resource)
	now := time.Now().UnixMilli()
	windowStart := now - windowSec*1000

	pipe := s.client.Pipeline()

	// Remove expired entries
	pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart))

	// Add current request (member = timestamp+random suffix for uniqueness)
	member := fmt.Sprintf("%d-%d", now, cost)
	for i := int64(0); i < cost; i++ {
		pipe.ZAdd(ctx, key, redis.Z{Score: float64(now), Member: fmt.Sprintf("%s-%d", member, i)})
	}

	// Count all entries in window
	pipe.ZCard(ctx, key)

	// Set expiry
	pipe.Expire(ctx, key, time.Duration(windowSec)*time.Second*2)

	cmds, err := pipe.Exec(ctx)
	if err != nil {
		return 0, err
	}

	// Last meaningful command is ZCard
	count := cmds[len(cmds)-2].(*redis.IntCmd).Val()
	return count, nil
}

func (s *RedisStore) SlidingWindowCount(ctx context.Context, clientID, resource string, windowSec int64) (int64, error) {
	key := fmt.Sprintf("rl:%s:%s", clientID, resource)
	now := time.Now().UnixMilli()
	windowStart := now - windowSec*1000

	// Remove stale then count
	pipe := s.client.Pipeline()
	pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart))
	pipe.ZCard(ctx, key)
	cmds, err := pipe.Exec(ctx)
	if err != nil {
		return 0, err
	}
	return cmds[1].(*redis.IntCmd).Val(), nil
}

func (s *RedisStore) ResetKey(ctx context.Context, clientID, resource string) error {
	key := fmt.Sprintf("rl:%s:%s", clientID, resource)
	return s.client.Del(ctx, key).Err()
}

func (s *RedisStore) Client() *redis.Client {
	return s.client
}