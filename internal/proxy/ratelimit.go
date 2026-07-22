package proxy

import (
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

const limiterShardCount = 64

type Limiter struct {
	rate         rate.Limit
	burst        int
	ttl          time.Duration
	maxEntries   int
	cleanupEvery time.Duration
	now          func() time.Time
	shards       [limiterShardCount]limiterShard
	entries      atomic.Int64
	lastCleanup  atomic.Int64
	cleaning     atomic.Bool
	evictionMu   sync.Mutex
}

type limiterShard struct {
	limiters map[string]*clientLimiter
	mu       sync.Mutex
}

type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func NewLimiter(r rate.Limit, b int) *Limiter {
	return newLimiter(r, b, 10*time.Minute, 10_000, time.Now)
}

func newLimiter(r rate.Limit, b int, ttl time.Duration, maxEntries int, now func() time.Time) *Limiter {
	if maxEntries < 1 {
		maxEntries = 1
	}
	limiter := &Limiter{
		rate:         r,
		burst:        b,
		ttl:          ttl,
		maxEntries:   maxEntries,
		cleanupEvery: time.Minute,
		now:          now,
	}
	for index := range limiter.shards {
		limiter.shards[index].limiters = make(map[string]*clientLimiter)
	}
	return limiter
}

func (l *Limiter) get(ip string) *rate.Limiter {
	now := l.now()
	l.cleanupExpired(now)

	shard := &l.shards[limiterShardIndex(ip)]
	shard.mu.Lock()
	if entry, exists := shard.limiters[ip]; exists {
		entry.lastSeen = now
		shard.mu.Unlock()
		return entry.limiter
	}
	shard.mu.Unlock()

	for !l.reserveEntry() {
		l.evictOldest()
	}

	shard.mu.Lock()
	if entry, exists := shard.limiters[ip]; exists {
		entry.lastSeen = now
		shard.mu.Unlock()
		l.entries.Add(-1)
		return entry.limiter
	}
	limiter := rate.NewLimiter(l.rate, l.burst)
	shard.limiters[ip] = &clientLimiter{limiter: limiter, lastSeen: now}
	shard.mu.Unlock()
	return limiter
}

func (l *Limiter) reserveEntry() bool {
	for {
		entries := l.entries.Load()
		if entries >= int64(l.maxEntries) {
			return false
		}
		if l.entries.CompareAndSwap(entries, entries+1) {
			return true
		}
	}
}

func (l *Limiter) cleanupExpired(now time.Time) {
	lastCleanup := time.Unix(0, l.lastCleanup.Load())
	if l.cleanupEvery > 0 && now.Sub(lastCleanup) < l.cleanupEvery {
		return
	}
	if !l.cleaning.CompareAndSwap(false, true) {
		return
	}
	defer l.cleaning.Store(false)

	lastCleanup = time.Unix(0, l.lastCleanup.Load())
	if l.cleanupEvery > 0 && now.Sub(lastCleanup) < l.cleanupEvery {
		return
	}
	for index := range l.shards {
		shard := &l.shards[index]
		shard.mu.Lock()
		for ip, entry := range shard.limiters {
			if now.Sub(entry.lastSeen) >= l.ttl {
				delete(shard.limiters, ip)
				l.entries.Add(-1)
			}
		}
		shard.mu.Unlock()
	}
	l.lastCleanup.Store(now.UnixNano())
}

func (l *Limiter) evictOldest() {
	l.evictionMu.Lock()
	defer l.evictionMu.Unlock()

	var oldestIP string
	var oldestTime time.Time
	var oldestShard *limiterShard
	for index := range l.shards {
		shard := &l.shards[index]
		shard.mu.Lock()
		for ip, entry := range shard.limiters {
			if oldestIP == "" || entry.lastSeen.Before(oldestTime) {
				oldestIP = ip
				oldestTime = entry.lastSeen
				oldestShard = shard
			}
		}
		shard.mu.Unlock()
	}
	if oldestShard == nil {
		return
	}
	oldestShard.mu.Lock()
	if _, exists := oldestShard.limiters[oldestIP]; exists {
		delete(oldestShard.limiters, oldestIP)
		l.entries.Add(-1)
	}
	oldestShard.mu.Unlock()
}

func limiterShardIndex(ip string) uint64 {
	var hash uint64 = 14695981039346656037
	for index := range len(ip) {
		hash ^= uint64(ip[index])
		hash *= 1099511628211
	}
	return hash & (limiterShardCount - 1)
}

func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r.RemoteAddr)
		if !l.get(ip).Allow() {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(remoteAddr string) string {
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil && host != "" {
		return host
	}
	if net.ParseIP(remoteAddr) != nil {
		return remoteAddr
	}
	return "invalid:" + remoteAddr
}
