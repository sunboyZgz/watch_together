package transport

import (
	"hash/fnv"
	"sync"
	"time"
)

const (
	defaultControlRateLimitShards     = 64
	defaultControlRateLimitMaxEntries = 100_000
	controlRateLimitCleanupEvery      = 256
)

type controlRateLimiter struct {
	interval         time.Duration
	maxEntriesPerSet int
	shards           []controlRateLimitShard
}

type controlRateLimitShard struct {
	mu         sync.Mutex
	entries    map[string]time.Time
	operations uint64
}

func newControlRateLimiter(interval time.Duration, maxEntries int, shardCount int) *controlRateLimiter {
	if interval <= 0 {
		return nil
	}
	if maxEntries <= 0 {
		maxEntries = defaultControlRateLimitMaxEntries
	}
	if shardCount <= 0 {
		shardCount = defaultControlRateLimitShards
	}
	maxEntriesPerSet := maxEntries / shardCount
	if maxEntriesPerSet <= 0 {
		maxEntriesPerSet = 1
	}
	return &controlRateLimiter{
		interval:         interval,
		maxEntriesPerSet: maxEntriesPerSet,
		shards:           make([]controlRateLimitShard, shardCount),
	}
}

func (l *controlRateLimiter) Reserve(key string, now time.Time) bool {
	if l == nil || l.interval <= 0 {
		return true
	}
	shard := l.shardFor(key)

	shard.mu.Lock()
	defer shard.mu.Unlock()

	if shard.entries == nil {
		shard.entries = make(map[string]time.Time)
	}
	shard.operations++

	if lastAcceptedAt, ok := shard.entries[key]; ok && now.Sub(lastAcceptedAt) < l.interval {
		return false
	}
	if shard.operations%controlRateLimitCleanupEvery == 0 || len(shard.entries) >= l.maxEntriesPerSet {
		cleanupExpiredControlRateEntries(shard.entries, now, l.interval)
	}
	if len(shard.entries) >= l.maxEntriesPerSet {
		return true
	}
	shard.entries[key] = now
	return true
}

func (l *controlRateLimiter) ForgetReservation(key string, reservedAt time.Time) {
	if l == nil || reservedAt.IsZero() {
		return
	}
	shard := l.shardFor(key)

	shard.mu.Lock()
	defer shard.mu.Unlock()

	if current, ok := shard.entries[key]; ok && current.Equal(reservedAt) {
		delete(shard.entries, key)
	}
}

func (l *controlRateLimiter) shardFor(key string) *controlRateLimitShard {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(key))
	return &l.shards[int(hash.Sum32())%len(l.shards)]
}

func cleanupExpiredControlRateEntries(entries map[string]time.Time, now time.Time, interval time.Duration) {
	for key, lastAcceptedAt := range entries {
		if now.Sub(lastAcceptedAt) >= interval {
			delete(entries, key)
		}
	}
}
