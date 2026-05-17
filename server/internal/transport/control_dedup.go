package transport

import (
	"hash/fnv"
	"sync"
	"time"
)

const (
	defaultControlRequestDedupShards     = 64
	defaultControlRequestDedupMaxEntries = 100_000
	controlRequestDedupCleanupEvery      = 256
)

type controlRequestDeduper struct {
	ttl              time.Duration
	maxEntriesPerSet int
	shards           []controlRequestDedupShard
}

// observe open-fail strategy
// use a sharded map to reduce lock contention
type controlRequestDedupShard struct {
	mu         sync.Mutex
	entries    map[string]time.Time
	operations uint64
}

func newControlRequestDeduper(ttl time.Duration, maxEntries int, shardCount int) *controlRequestDeduper {
	if ttl <= 0 {
		return nil
	}
	if maxEntries <= 0 {
		maxEntries = defaultControlRequestDedupMaxEntries
	}
	if shardCount <= 0 {
		shardCount = defaultControlRequestDedupShards
	}
	maxEntriesPerSet := maxEntries / shardCount
	if maxEntriesPerSet <= 0 {
		maxEntriesPerSet = 1
	}
	return &controlRequestDeduper{
		ttl:              ttl,
		maxEntriesPerSet: maxEntriesPerSet,
		shards:           make([]controlRequestDedupShard, shardCount),
	}
}

// Reserve returns false when the same room/request pair was accepted recently.
// If the local dedup set is saturated, it fails open: the control can proceed,
// but this requestId may not be remembered for a future duplicate.
func (d *controlRequestDeduper) Reserve(roomID string, requestID string, now time.Time) bool {
	if d == nil || requestID == "" || d.ttl <= 0 {
		return true
	}
	key := controlRequestDedupKey(roomID, requestID)
	shard := d.shardFor(key)

	shard.mu.Lock()
	defer shard.mu.Unlock()

	if shard.entries == nil {
		shard.entries = make(map[string]time.Time)
	}
	shard.operations++

	if expiresAt, ok := shard.entries[key]; ok && expiresAt.After(now) {
		return false
	}
	if shard.operations%controlRequestDedupCleanupEvery == 0 || len(shard.entries) >= d.maxEntriesPerSet {
		cleanupExpiredControlRequests(shard.entries, now)
	}
	if len(shard.entries) >= d.maxEntriesPerSet {
		return true
	}
	shard.entries[key] = now.Add(d.ttl)
	return true
}

func (d *controlRequestDeduper) Forget(roomID string, requestID string) {
	if d == nil || requestID == "" {
		return
	}
	key := controlRequestDedupKey(roomID, requestID)
	shard := d.shardFor(key)

	shard.mu.Lock()
	defer shard.mu.Unlock()

	delete(shard.entries, key)
}

func (d *controlRequestDeduper) shardFor(key string) *controlRequestDedupShard {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(key))
	return &d.shards[int(hash.Sum32())%len(d.shards)]
}

func cleanupExpiredControlRequests(entries map[string]time.Time, now time.Time) {
	for key, expiresAt := range entries {
		if !expiresAt.After(now) {
			delete(entries, key)
		}
	}
}

func controlRequestDedupKey(roomID string, requestID string) string {
	return roomID + "\x00" + requestID
}
