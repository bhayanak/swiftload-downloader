package engine

import (
	"context"
	"sync"
	"time"
)

// rateLimiter is a simple token-bucket limiter shared across chunk workers to
// cap the aggregate download rate. A nil *rateLimiter means unlimited.
type rateLimiter struct {
	mu         sync.Mutex
	rate       int64 // bytes per second
	capacity   int64
	tokens     float64
	lastRefill time.Time
}

// newRateLimiter builds a limiter for bytesPerSec. Returns nil if bytesPerSec<=0.
func newRateLimiter(bytesPerSec int64) *rateLimiter {
	if bytesPerSec <= 0 {
		return nil
	}
	return &rateLimiter{
		rate:       bytesPerSec,
		capacity:   bytesPerSec, // allow up to 1s burst
		tokens:     float64(bytesPerSec),
		lastRefill: time.Now(),
	}
}

// wait blocks until n tokens (bytes) are available or the context is done.
func (r *rateLimiter) wait(ctx context.Context, n int64) {
	if r == nil {
		return
	}
	for {
		r.mu.Lock()
		now := time.Now()
		elapsed := now.Sub(r.lastRefill).Seconds()
		r.lastRefill = now
		r.tokens += elapsed * float64(r.rate)
		if r.tokens > float64(r.capacity) {
			r.tokens = float64(r.capacity)
		}
		if r.tokens >= float64(n) {
			r.tokens -= float64(n)
			r.mu.Unlock()
			return
		}
		deficit := float64(n) - r.tokens
		wait := time.Duration(deficit / float64(r.rate) * float64(time.Second))
		r.mu.Unlock()

		if wait <= 0 {
			wait = time.Millisecond
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}
