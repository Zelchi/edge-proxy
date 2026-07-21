package proxy

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type Limiter struct {
	limiters     map[string]*clientLimiter
	mu           sync.Mutex
	rate         rate.Limit
	burst        int
	ttl          time.Duration
	maxEntries   int
	lastCleanup  time.Time
	cleanupEvery time.Duration
	now          func() time.Time
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
	return &Limiter{
		limiters:     make(map[string]*clientLimiter),
		rate:         r,
		burst:        b,
		ttl:          ttl,
		maxEntries:   maxEntries,
		cleanupEvery: time.Minute,
		now:          now,
	}
}

func (l *Limiter) get(ip string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	if now.Sub(l.lastCleanup) >= l.cleanupEvery {
		l.cleanupExpired(now)
		l.lastCleanup = now
	}

	if entry, exists := l.limiters[ip]; exists {
		entry.lastSeen = now
		return entry.limiter
	}

	if len(l.limiters) >= l.maxEntries {
		l.evictOldest()
	}
	limiter := rate.NewLimiter(l.rate, l.burst)
	l.limiters[ip] = &clientLimiter{limiter: limiter, lastSeen: now}
	return limiter
}

func (l *Limiter) cleanupExpired(now time.Time) {
	for ip, entry := range l.limiters {
		if now.Sub(entry.lastSeen) >= l.ttl {
			delete(l.limiters, ip)
		}
	}
}

func (l *Limiter) evictOldest() {
	var oldestIP string
	var oldestTime time.Time
	for ip, entry := range l.limiters {
		if oldestIP == "" || entry.lastSeen.Before(oldestTime) {
			oldestIP = ip
			oldestTime = entry.lastSeen
		}
	}
	delete(l.limiters, oldestIP)
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
