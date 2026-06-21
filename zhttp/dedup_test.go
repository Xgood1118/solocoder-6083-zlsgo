package zhttp_test

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sohaha/zlsgo"
	"github.com/sohaha/zlsgo/zhttp"
)

func TestDedupBasic(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	var callCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&callCount, 1)
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	e := zhttp.New()
	e.EnableDedup(zhttp.DedupConfig{
		Window: 5 * time.Second,
	})

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := e.Get(server.URL)
			t.NoError(err)
			t.Equal(200, resp.StatusCode())
			t.Equal("ok", resp.String())
		}()
	}
	wg.Wait()

	t.Equal(int64(1), atomic.LoadInt64(&callCount))
}

func TestDedupDifferentURLs(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	var callCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&callCount, 1)
		w.WriteHeader(200)
		w.Write([]byte(r.URL.Path))
	}))
	defer server.Close()

	e := zhttp.New()
	e.EnableDedup(zhttp.DedupConfig{
		Window: 5 * time.Second,
	})

	resp1, err := e.Get(server.URL + "/a")
	t.NoError(err)
	t.Equal("/a", resp1.String())

	resp2, err := e.Get(server.URL + "/b")
	t.NoError(err)
	t.Equal("/b", resp2.String())

	t.Equal(int64(2), atomic.LoadInt64(&callCount))
}

func TestDedupDifferentMethods(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	var callCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&callCount, 1)
		w.WriteHeader(200)
		w.Write([]byte(r.Method))
	}))
	defer server.Close()

	e := zhttp.New()
	e.EnableDedup(zhttp.DedupConfig{
		Window: 5 * time.Second,
	})

	resp1, err := e.Get(server.URL)
	t.NoError(err)
	t.Equal("GET", resp1.String())

	resp2, err := e.Post(server.URL)
	t.NoError(err)
	t.Equal("POST", resp2.String())

	t.Equal(int64(2), atomic.LoadInt64(&callCount))
}

func TestDedupWithBody(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	var callCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&callCount, 1)
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	e := zhttp.New()
	e.EnableDedup(zhttp.DedupConfig{
		Window: 5 * time.Second,
	})

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := e.Post(server.URL, zhttp.BodyJSON(map[string]interface{}{"data": "same"}))
			t.NoError(err)
			t.Equal(200, resp.StatusCode())
		}()
	}
	wg.Wait()

	t.Equal(int64(1), atomic.LoadInt64(&callCount))
}

func TestDedupDifferentBody(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	var callCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&callCount, 1)
		w.WriteHeader(200)
	}))
	defer server.Close()

	e := zhttp.New()
	e.EnableDedup(zhttp.DedupConfig{
		Window: 5 * time.Second,
	})

	_, err := e.Post(server.URL, zhttp.BodyJSON(map[string]interface{}{"data": "a"}))
	t.NoError(err)

	_, err = e.Post(server.URL, zhttp.BodyJSON(map[string]interface{}{"data": "b"}))
	t.NoError(err)

	t.Equal(int64(2), atomic.LoadInt64(&callCount))
}

func TestDedupWindowExpire(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	var callCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&callCount, 1)
		w.WriteHeader(200)
	}))
	defer server.Close()

	e := zhttp.New()
	e.EnableDedup(zhttp.DedupConfig{
		Window: 100 * time.Millisecond,
	})

	_, err := e.Get(server.URL)
	t.NoError(err)
	t.Equal(int64(1), atomic.LoadInt64(&callCount))

	_, err = e.Get(server.URL)
	t.NoError(err)
	t.Equal(int64(1), atomic.LoadInt64(&callCount))

	time.Sleep(200 * time.Millisecond)

	_, err = e.Get(server.URL)
	t.NoError(err)
	t.Equal(int64(2), atomic.LoadInt64(&callCount))
}

func TestDedupDisable(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	var callCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&callCount, 1)
		w.WriteHeader(200)
	}))
	defer server.Close()

	e := zhttp.New()
	e.EnableDedup(zhttp.DedupConfig{
		Window: 5 * time.Second,
	})

	e.DisableDedup()

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.Get(server.URL)
			t.NoError(err)
		}()
	}
	wg.Wait()

	t.Equal(int64(5), atomic.LoadInt64(&callCount))
}

func TestDedupClear(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	var callCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&callCount, 1)
		w.WriteHeader(200)
	}))
	defer server.Close()

	e := zhttp.New()
	e.EnableDedup(zhttp.DedupConfig{
		Window: 5 * time.Second,
	})

	_, err := e.Get(server.URL)
	t.NoError(err)
	t.Equal(int64(1), atomic.LoadInt64(&callCount))

	e.ClearDedup()

	_, err = e.Get(server.URL)
	t.NoError(err)
	t.Equal(int64(2), atomic.LoadInt64(&callCount))
}

func TestDedupResponseClone(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(30 * time.Millisecond)
		w.WriteHeader(200)
		w.Write([]byte("shared response"))
	}))
	defer server.Close()

	e := zhttp.New()
	e.EnableDedup(zhttp.DedupConfig{
		Window: 5 * time.Second,
	})

	var wg sync.WaitGroup
	var resps [3]*zhttp.Res

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			resp, err := e.Get(server.URL)
			t.NoError(err)
			resps[idx] = resp
		}(i)
	}
	wg.Wait()

	for i := 0; i < 3; i++ {
		t.Equal("shared response", resps[i].String())
	}

	different := false
	for i := 0; i < 2; i++ {
		for j := i + 1; j < 3; j++ {
			if resps[i] != resps[j] {
				different = true
			}
		}
	}
	t.EqualTrue(different)
}

func TestDedupConcurrent(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	var callCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&callCount, 1)
		time.Sleep(20 * time.Millisecond)
		w.WriteHeader(200)
	}))
	defer server.Close()

	e := zhttp.New()
	e.EnableDedup(zhttp.DedupConfig{
		Window: 5 * time.Second,
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := e.Get(server.URL)
			t.NoError(err)
			t.Equal(200, resp.StatusCode())
		}()
	}
	wg.Wait()

	t.EqualTrue(atomic.LoadInt64(&callCount) < 10)
}

func TestDedupGlobalFunctions(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	var callCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&callCount, 1)
		time.Sleep(30 * time.Millisecond)
		w.WriteHeader(200)
	}))
	defer server.Close()

	zhttp.EnableDedup(zhttp.DedupConfig{
		Window: 5 * time.Second,
	})
	defer zhttp.DisableDedup()

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := zhttp.Get(server.URL)
			t.NoError(err)
			t.Equal(200, resp.StatusCode())
		}()
	}
	wg.Wait()

	t.EqualTrue(atomic.LoadInt64(&callCount) == 1)
}

func TestDedupErrorSharing(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	var callCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&callCount, 1)
		time.Sleep(30 * time.Millisecond)
		w.WriteHeader(500)
	}))
	defer server.Close()

	e := zhttp.New()
	e.EnableDedup(zhttp.DedupConfig{
		Window: 5 * time.Second,
	})

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := e.Get(server.URL)
			t.NoError(err)
			t.Equal(500, resp.StatusCode())
		}()
	}
	wg.Wait()

	t.Equal(int64(1), atomic.LoadInt64(&callCount))
}
