package httpx

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

// RateLimiter adalah token bucket per kunci, disimpan di memori.
// Cukup untuk satu instance (SPEC bagian 7); proxy tetap membatasi per IP.
type RateLimiter struct {
	mu      sync.Mutex
	rate    float64 // token per detik
	burst   float64
	buckets map[string]*bucket
	now     func() time.Time
	lastGC  time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

// NewRateLimiter membuat limiter dengan perMinute permintaan per menit per kunci.
func NewRateLimiter(perMinute int) *RateLimiter {
	return &RateLimiter{
		rate:    float64(perMinute) / 60,
		burst:   float64(perMinute),
		buckets: make(map[string]*bucket),
		now:     time.Now,
	}
}

// Allow mengembalikan true bila kunci masih punya kuota, beserta waktu tunggu bila tidak.
func (l *RateLimiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.gc(now)
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	b.tokens = min(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.rate)
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	wait := time.Duration((1 - b.tokens) / l.rate * float64(time.Second))
	return false, wait
}

// gc membuang bucket yang sudah penuh kembali agar map tidak tumbuh tanpa batas.
func (l *RateLimiter) gc(now time.Time) {
	if now.Sub(l.lastGC) < time.Minute {
		return
	}
	l.lastGC = now
	full := time.Duration(l.burst / l.rate * float64(time.Second))
	for k, b := range l.buckets {
		if now.Sub(b.last) > full {
			delete(l.buckets, k)
		}
	}
}

// LimitByIP membatasi permintaan per IP klien.
func LimitByIP(l *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ok, wait := l.Allow(IPFrom(r.Context()).String())
			if !ok {
				w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
				WriteError(w, http.StatusTooManyRequests, "rate_limited", "Terlalu banyak percobaan. Coba lagi nanti.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
