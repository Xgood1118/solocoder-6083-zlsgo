package limiter

import (
	"sync"
	"time"
)

type tokenBucket struct {
	mu         sync.Mutex
	capacity   int64
	tokens     int64
	rate       float64
	lastRefill time.Time
}

func newTokenBucket(rate float64, capacity int64) *tokenBucket {
	return &tokenBucket{
		capacity:   capacity,
		tokens:     capacity,
		rate:       rate,
		lastRefill: time.Now(),
	}
}

func (tb *tokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	if elapsed > 0 {
		tb.tokens += int64(elapsed * tb.rate)
		if tb.tokens > tb.capacity {
			tb.tokens = tb.capacity
		}
		tb.lastRefill = now
	}
}

func (tb *tokenBucket) allow() (bool, time.Duration) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens > 0 {
		tb.tokens--
		return true, 0
	}

	if tb.rate <= 0 {
		return false, time.Duration(1<<63 - 1)
	}

	retryAfter := time.Duration(float64(time.Second) / tb.rate)
	return false, retryAfter
}

type TokenBucketLimiter struct {
	mu       sync.RWMutex
	buckets  map[interface{}]*tokenBucket
	rate     float64
	capacity int64
}

func NewTokenBucketLimiter(rate float64, capacity int64) *TokenBucketLimiter {
	return &TokenBucketLimiter{
		buckets:  make(map[interface{}]*tokenBucket),
		rate:     rate,
		capacity: capacity,
	}
}

func (tbl *TokenBucketLimiter) Allow(key interface{}) (bool, time.Duration) {
	tbl.mu.RLock()
	bucket, ok := tbl.buckets[key]
	tbl.mu.RUnlock()

	if !ok {
		tbl.mu.Lock()
		bucket, ok = tbl.buckets[key]
		if !ok {
			bucket = newTokenBucket(tbl.rate, tbl.capacity)
			tbl.buckets[key] = bucket
		}
		tbl.mu.Unlock()
	}

	return bucket.allow()
}

func (tbl *TokenBucketLimiter) AllowGlobal() (bool, time.Duration) {
	return tbl.Allow("__global__")
}

func (tbl *TokenBucketLimiter) SetRate(rate float64) {
	tbl.mu.Lock()
	defer tbl.mu.Unlock()
	tbl.rate = rate
	for _, b := range tbl.buckets {
		b.mu.Lock()
		b.rate = rate
		b.mu.Unlock()
	}
}

func (tbl *TokenBucketLimiter) SetCapacity(capacity int64) {
	tbl.mu.Lock()
	defer tbl.mu.Unlock()
	tbl.capacity = capacity
	for _, b := range tbl.buckets {
		b.mu.Lock()
		b.capacity = capacity
		if b.tokens > capacity {
			b.tokens = capacity
		}
		b.mu.Unlock()
	}
}

func (tbl *TokenBucketLimiter) Reset(key interface{}) {
	tbl.mu.Lock()
	defer tbl.mu.Unlock()
	delete(tbl.buckets, key)
}

func (tbl *TokenBucketLimiter) ResetAll() {
	tbl.mu.Lock()
	defer tbl.mu.Unlock()
	tbl.buckets = make(map[interface{}]*tokenBucket)
}
