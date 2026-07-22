package proxy

import (
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestLimiterExpiresInactiveEntries(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	limiter := newLimiter(rate.Limit(1), 1, time.Minute, 10, func() time.Time { return now })
	limiter.cleanupEvery = 0
	limiter.get("old")

	now = now.Add(2 * time.Minute)
	limiter.get("new")

	if limiter.has("old") {
		t.Fatal("expired limiter entry was retained")
	}
	if entries := limiter.entryCount(); entries != 1 {
		t.Fatalf("entries = %d, want 1", entries)
	}
}

func TestLimiterEvictsOldestEntryAtCapacity(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	limiter := newLimiter(rate.Limit(1), 1, time.Hour, 2, func() time.Time { return now })
	limiter.get("first")
	now = now.Add(time.Second)
	limiter.get("second")
	now = now.Add(time.Second)
	limiter.get("third")

	if limiter.has("first") {
		t.Fatal("oldest limiter entry was retained at capacity")
	}
	if entries := limiter.entryCount(); entries != 2 {
		t.Fatalf("entries = %d, want 2", entries)
	}
}

func (l *Limiter) has(ip string) bool {
	shard := &l.shards[limiterShardIndex(ip)]
	shard.mu.Lock()
	defer shard.mu.Unlock()
	_, exists := shard.limiters[ip]
	return exists
}

func (l *Limiter) entryCount() int {
	return int(l.entries.Load())
}

func TestClientIPDoesNotUseAnEmptyKey(t *testing.T) {
	if got := clientIP("not-an-address"); got != "invalid:not-an-address" {
		t.Fatalf("clientIP() = %q", got)
	}
}
