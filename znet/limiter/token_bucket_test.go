package limiter_test

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sohaha/zlsgo"
	"github.com/sohaha/zlsgo/znet"
	"github.com/sohaha/zlsgo/znet/limiter"
)

func TestTokenBucketBasic(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	tb := limiter.NewTokenBucketLimiter(10, 5)

	for i := 0; i < 5; i++ {
		allowed, _ := tb.Allow("test")
		t.EqualTrue(allowed)
	}

	allowed, retryAfter := tb.Allow("test")
	t.EqualFalse(allowed)
	t.EqualTrue(retryAfter > 0)
}

func TestTokenBucketRefill(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	tb := limiter.NewTokenBucketLimiter(100, 2)

	allowed, _ := tb.Allow("refill")
	t.EqualTrue(allowed)

	allowed, _ = tb.Allow("refill")
	t.EqualTrue(allowed)

	allowed, _ = tb.Allow("refill")
	t.EqualFalse(allowed)

	time.Sleep(20 * time.Millisecond)

	allowed, _ = tb.Allow("refill")
	t.EqualTrue(allowed)
}

func TestTokenBucketGlobal(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	tb := limiter.NewTokenBucketLimiter(10, 3)

	for i := 0; i < 3; i++ {
		allowed, _ := tb.AllowGlobal()
		t.EqualTrue(allowed)
	}

	allowed, _ := tb.AllowGlobal()
	t.EqualFalse(allowed)
}

func TestTokenBucketMultipleKeys(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	tb := limiter.NewTokenBucketLimiter(10, 2)

	for i := 0; i < 2; i++ {
		allowed, _ := tb.Allow("key1")
		t.EqualTrue(allowed)
	}
	allowed, _ := tb.Allow("key1")
	t.EqualFalse(allowed)

	for i := 0; i < 2; i++ {
		allowed, _ := tb.Allow("key2")
		t.EqualTrue(allowed)
	}
	allowed, _ = tb.Allow("key2")
	t.EqualFalse(allowed)
}

func TestTokenBucketReset(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	tb := limiter.NewTokenBucketLimiter(10, 1)

	allowed, _ := tb.Allow("reset")
	t.EqualTrue(allowed)

	allowed, _ = tb.Allow("reset")
	t.EqualFalse(allowed)

	tb.Reset("reset")

	allowed, _ = tb.Allow("reset")
	t.EqualTrue(allowed)
}

func TestTokenBucketResetAll(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	tb := limiter.NewTokenBucketLimiter(10, 1)

	tb.Allow("a")
	tb.Allow("b")

	_, ra := tb.Allow("a")
	t.EqualTrue(ra > 0)
	_, rb := tb.Allow("b")
	t.EqualTrue(rb > 0)

	tb.ResetAll()

	allowed, _ := tb.Allow("a")
	t.EqualTrue(allowed)
	allowed, _ = tb.Allow("b")
	t.EqualTrue(allowed)
}

func TestTokenBucketMiddlewareGlobal(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	r := znet.New("tb_global_test")
	mw := limiter.NewTokenBucketMiddleware(10, 3)

	r.GET("/global", func(c *znet.Context) {
		c.String(200, "ok")
	}, mw.Global())

	var success int64
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/global", nil)
			req.Header.Set("X-Real-Ip", "10.0.0.1")
			r.ServeHTTP(w, req)
			if w.Code == http.StatusOK {
				atomic.AddInt64(&success, 1)
			}
		}()
	}
	wg.Wait()

	t.EqualTrue(success == 3)
}

func TestTokenBucketMiddlewareGroup(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	r := znet.New("tb_group_test")
	mw := limiter.NewTokenBucketMiddleware(0, 0).
		AddGroup("api", 10, 2)

	r.GET("/api/test", func(c *znet.Context) {
		c.String(200, "ok")
	}, mw.Group("api"))

	r.GET("/other", func(c *znet.Context) {
		c.String(200, "ok")
	})

	var wg sync.WaitGroup
	var apiSuccess, otherSuccess int64

	for i := 0; i < 5; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/api/test", nil)
			req.Header.Set("X-Real-Ip", "10.0.0.2")
			r.ServeHTTP(w, req)
			if w.Code == http.StatusOK {
				atomic.AddInt64(&apiSuccess, 1)
			}
		}()
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/other", nil)
			r.ServeHTTP(w, req)
			if w.Code == http.StatusOK {
				atomic.AddInt64(&otherSuccess, 1)
			}
		}()
	}
	wg.Wait()

	t.EqualTrue(apiSuccess == 2)
	t.EqualTrue(otherSuccess == 5)
}

func TestTokenBucketMiddlewareRetryAfter(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	r := znet.New("tb_retry_test")
	mw := limiter.NewTokenBucketMiddleware(1, 1)

	r.GET("/retry", func(c *znet.Context) {
		c.String(200, "ok")
	}, mw.Global())

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/retry", nil)
	r.ServeHTTP(w, req)
	t.Equal(200, w.Code)

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/retry", nil)
	r.ServeHTTP(w, req)
	t.Equal(429, w.Code)
	t.EqualTrue(w.Header().Get("Retry-After") != "")
}

func TestTokenBucketMiddlewareCustomOverflow(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	r := znet.New("tb_custom_test")
	mw := limiter.NewTokenBucketMiddleware(1, 1).
		SetOverflowHandler(func(c *znet.Context, retryAfter time.Duration) {
			c.SetHeader("X-Custom-Retry", "true", true)
			c.String(429, "custom rate limit")
		})

	r.GET("/custom", func(c *znet.Context) {
		c.String(200, "ok")
	}, mw.Global())

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/custom", nil)
	r.ServeHTTP(w, req)
	t.Equal(200, w.Code)

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/custom", nil)
	r.ServeHTTP(w, req)
	t.Equal(429, w.Code)
	t.Equal("true", w.Header().Get("X-Custom-Retry"))
	t.Equal("custom rate limit", w.Body.String())
}

func TestTokenBucketConcurrent(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	tb := limiter.NewTokenBucketLimiter(100, 100)

	var wg sync.WaitGroup
	var success, fail int64

	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed, _ := tb.Allow("concurrent")
			if allowed {
				atomic.AddInt64(&success, 1)
			} else {
				atomic.AddInt64(&fail, 1)
			}
		}()
	}
	wg.Wait()

	t.Equal(int64(100), success)
	t.Equal(int64(100), fail)
}
