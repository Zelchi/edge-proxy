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

	if _, exists := limiter.limiters["old"]; exists {
		t.Fatal("expired limiter entry was retained")
	}
	if len(limiter.limiters) != 1 {
		t.Fatalf("entries = %d, want 1", len(limiter.limiters))
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

	if _, exists := limiter.limiters["first"]; exists {
		t.Fatal("oldest limiter entry was retained at capacity")
	}
	if len(limiter.limiters) != 2 {
		t.Fatalf("entries = %d, want 2", len(limiter.limiters))
	}
}

func TestClientIPDoesNotUseAnEmptyKey(t *testing.T) {
	if got := clientIP("not-an-address"); got != "invalid:not-an-address" {
		t.Fatalf("clientIP() = %q", got)
	}
}
