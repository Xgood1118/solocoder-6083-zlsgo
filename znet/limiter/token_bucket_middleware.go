package limiter

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/sohaha/zlsgo/znet"
)

type TokenBucketMiddleware struct {
	globalLimiter *TokenBucketLimiter
	groupLimiters map[string]*TokenBucketLimiter
	keyFunc       func(c *znet.Context) interface{}
	overflow      func(c *znet.Context, retryAfter time.Duration)
}

func NewTokenBucketMiddleware(globalRate float64, globalCapacity int64) *TokenBucketMiddleware {
	m := &TokenBucketMiddleware{
		groupLimiters: make(map[string]*TokenBucketLimiter),
		keyFunc: func(c *znet.Context) interface{} {
			return "__shared__"
		},
		overflow: defaultOverflowHandler,
	}
	if globalRate > 0 && globalCapacity > 0 {
		m.globalLimiter = NewTokenBucketLimiter(globalRate, globalCapacity)
	}
	return m
}

func NewTokenBucket(rate float64, capacity int64, overflow ...func(c *znet.Context)) znet.HandlerFunc {
	m := NewTokenBucketMiddleware(rate, capacity)
	if len(overflow) > 0 {
		fn := overflow[0]
		m.SetOverflowHandler(func(c *znet.Context, d time.Duration) {
			fn(c)
		})
	}
	return m.Global()
}

func NewTokenBucketByIP(rate float64, capacity int64, overflow ...func(c *znet.Context)) znet.HandlerFunc {
	m := NewTokenBucketMiddleware(rate, capacity)
	m.SetKeyFunc(func(c *znet.Context) interface{} {
		return c.GetClientIP()
	})
	if len(overflow) > 0 {
		fn := overflow[0]
		m.SetOverflowHandler(func(c *znet.Context, d time.Duration) {
			fn(c)
		})
	}
	return m.Global()
}

func defaultOverflowHandler(c *znet.Context, retryAfter time.Duration) {
	retrySeconds := int64(retryAfter.Seconds())
	if retrySeconds < 1 {
		retrySeconds = 1
	}
	c.SetHeader("Retry-After", strconv.FormatInt(retrySeconds, 10), true)
	c.String(http.StatusTooManyRequests, http.StatusText(http.StatusTooManyRequests))
}

func (m *TokenBucketMiddleware) SetKeyFunc(fn func(c *znet.Context) interface{}) *TokenBucketMiddleware {
	m.keyFunc = fn
	return m
}

func (m *TokenBucketMiddleware) SetOverflowHandler(fn func(c *znet.Context, retryAfter time.Duration)) *TokenBucketMiddleware {
	m.overflow = fn
	return m
}

func (m *TokenBucketMiddleware) AddGroup(name string, rate float64, capacity int64) *TokenBucketMiddleware {
	m.groupLimiters[name] = NewTokenBucketLimiter(rate, capacity)
	return m
}

func (m *TokenBucketMiddleware) Group(name string) znet.HandlerFunc {
	return func(c *znet.Context) {
		limiter, ok := m.groupLimiters[name]
		if !ok {
			c.Next()
			return
		}

		key := m.keyFunc(c)
		if key == nil {
			key = "__shared__"
		}

		allowed, retryAfter := limiter.Allow(fmt.Sprintf("%s:%v", name, key))
		if !allowed {
			m.overflow(c, retryAfter)
			c.Abort()
			return
		}

		c.Next()
	}
}

func (m *TokenBucketMiddleware) Global() znet.HandlerFunc {
	return func(c *znet.Context) {
		if m.globalLimiter == nil {
			c.Next()
			return
		}

		key := m.keyFunc(c)
		if key == nil {
			key = "__shared__"
		}

		allowed, retryAfter := m.globalLimiter.Allow(fmt.Sprintf("__global__:%v", key))
		if !allowed {
			m.overflow(c, retryAfter)
			c.Abort()
			return
		}

		c.Next()
	}
}

func (m *TokenBucketMiddleware) Handler() znet.HandlerFunc {
	return func(c *znet.Context) {
		if m.globalLimiter != nil {
			key := m.keyFunc(c)
			if key == nil {
				key = "__shared__"
			}
			allowed, retryAfter := m.globalLimiter.Allow(fmt.Sprintf("__global__:%v", key))
			if !allowed {
				m.overflow(c, retryAfter)
				c.Abort()
				return
			}
		}
		c.Next()
	}
}
