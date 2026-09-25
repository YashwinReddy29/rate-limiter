package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// One script serializes every decision for a key. Only admitted units are stored.
// Redis TIME avoids disagreement between application host clocks. Random members
// prevent collisions between processes and requests in the same millisecond.
var windowScript = redis.NewScript(`
local t = redis.call('TIME')
local now = tonumber(t[1])*1000 + math.floor(tonumber(t[2])/1000)
local window = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
local cost = tonumber(ARGV[3])
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now-window)
local used = redis.call('ZCARD', KEYS[1])
local allowed = 0
if cost > 0 and used + cost <= limit then
  for i=1,cost do
    redis.call('ZADD', KEYS[1], now, ARGV[4] .. ':' .. i)
  end
  used = used + cost
  allowed = 1
  redis.call('PEXPIRE', KEYS[1], window)
end
local reset = 0
local last = redis.call('ZREVRANGE', KEYS[1], 0, 0, 'WITHSCORES')
if #last > 0 then reset = math.max(0, tonumber(last[2])+window-now) end
return {allowed, used, reset}
`)

type RedisStore struct{ client *redis.Client }
type Decision struct {
	Allowed            bool
	Used, ResetAfterMs int64
}

func NewRedisStore(addr string) *RedisStore {
	return NewWithOptions(&redis.Options{Addr: addr})
}
func NewWithOptions(opts *redis.Options) *RedisStore {
	// A timeout after execution is ambiguous: retrying a mutation can consume twice.
	opts.MaxRetries = -1
	opts.DialTimeout = time.Second
	opts.ReadTimeout = time.Second
	opts.WriteTimeout = time.Second
	opts.PoolTimeout = time.Second
	opts.ContextTimeoutEnabled = true
	opts.PoolSize = 50
	return &RedisStore{client: redis.NewClient(opts)}
}
func key(clientID, resource string) string {
	sum := sha256.Sum256([]byte(clientID + "\x00" + resource))
	return "rl:v2:" + hex.EncodeToString(sum[:])
}
func (s *RedisStore) Window(ctx context.Context, clientID, resource string, windowSec, limit, cost int64) (Decision, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return Decision{}, err
	}
	values, err := windowScript.Run(ctx, s.client, []string{key(clientID, resource)}, windowSec*1000, limit, cost, hex.EncodeToString(nonce[:])).Int64Slice()
	if err != nil {
		return Decision{}, fmt.Errorf("window decision: %w", err)
	}
	if len(values) != 3 {
		return Decision{}, fmt.Errorf("invalid script response")
	}
	return Decision{Allowed: values[0] == 1, Used: values[1], ResetAfterMs: values[2]}, nil
}
func (s *RedisStore) Ping(ctx context.Context) error { return s.client.Ping(ctx).Err() }
func (s *RedisStore) ResetKey(ctx context.Context, clientID, resource string) error {
	return s.client.Del(ctx, key(clientID, resource)).Err()
}
func (s *RedisStore) Close() error { return s.client.Close() }
