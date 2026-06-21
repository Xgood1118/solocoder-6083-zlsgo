package zhttp

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"sync"
	"time"

	"github.com/sohaha/zlsgo/zstring"
)

type dedupEntry struct {
	mu       sync.Mutex
	cond     *sync.Cond
	done     bool
	res      *Res
	err      error
	expireAt time.Time
}

type DedupConfig struct {
	Window       time.Duration
	MaxCacheSize int
	Enabled      bool
}

type RequestDeduplicator struct {
	mu       sync.RWMutex
	entries  map[string]*dedupEntry
	config   DedupConfig
	hasher   sync.Pool
}

func NewRequestDeduplicator(config ...DedupConfig) *RequestDeduplicator {
	cfg := DedupConfig{
		Window:       5 * time.Second,
		MaxCacheSize: 1000,
		Enabled:      true,
	}
	if len(config) > 0 {
		if config[0].Window > 0 {
			cfg.Window = config[0].Window
		}
		if config[0].MaxCacheSize > 0 {
			cfg.MaxCacheSize = config[0].MaxCacheSize
		}
		cfg.Enabled = config[0].Enabled
	}
	return &RequestDeduplicator{
		entries: make(map[string]*dedupEntry),
		config:  cfg,
		hasher: sync.Pool{
			New: func() interface{} {
				return sha256.New()
			},
		},
	}
}

func (rd *RequestDeduplicator) SetEnabled(enabled bool) {
	rd.mu.Lock()
	rd.config.Enabled = enabled
	rd.mu.Unlock()
}

func (rd *RequestDeduplicator) SetWindow(window time.Duration) {
	rd.mu.Lock()
	rd.config.Window = window
	rd.mu.Unlock()
}

func (rd *RequestDeduplicator) computeHash(method, url string, body []byte) string {
	h := rd.hasher.Get().(hash.Hash)
	defer func() {
		h.Reset()
		rd.hasher.Put(h)
	}()

	h.Write(zstring.String2Bytes(method))
	h.Write(zstring.String2Bytes("|"))
	h.Write(zstring.String2Bytes(url))
	if body != nil && len(body) > 0 {
		h.Write(zstring.String2Bytes("|"))
		h.Write(body)
	}

	sum := h.Sum(nil)
	return hex.EncodeToString(sum)
}

func (rd *RequestDeduplicator) Do(
	method, rawurl string,
	body []byte,
	fn func() (*Res, error),
) (*Res, error) {
	if !rd.config.Enabled {
		return fn()
	}

	hash := rd.computeHash(method, rawurl, body)

	rd.mu.RLock()
	entry, exists := rd.entries[hash]
	if exists && time.Now().After(entry.expireAt) {
		rd.mu.RUnlock()
		rd.mu.Lock()
		if e, ok := rd.entries[hash]; ok && time.Now().After(e.expireAt) {
			delete(rd.entries, hash)
			exists = false
			entry = nil
		}
		rd.mu.Unlock()
		rd.mu.RLock()
	}
	rd.mu.RUnlock()

	if exists {
		entry.mu.Lock()
		if entry.done {
			res, err := cloneRes(entry.res), entry.err
			entry.mu.Unlock()
			return res, err
		}
		for !entry.done {
			entry.cond.Wait()
		}
		res, err := cloneRes(entry.res), entry.err
		entry.mu.Unlock()
		return res, err
	}

	rd.mu.Lock()
	entry, exists = rd.entries[hash]
	if !exists {
		entry = &dedupEntry{
			expireAt: time.Now().Add(rd.config.Window),
		}
		entry.cond = sync.NewCond(&entry.mu)
		rd.entries[hash] = entry
		rd.cleanup()
	}
	rd.mu.Unlock()

	entry.mu.Lock()
	if entry.done {
		res, err := cloneRes(entry.res), entry.err
		entry.mu.Unlock()
		return res, err
	}
	entry.mu.Unlock()

	res, err := fn()

	entry.mu.Lock()
	entry.res = res
	entry.err = err
	entry.done = true
	entry.cond.Broadcast()
	entry.mu.Unlock()

	return cloneRes(res), err
}

func (rd *RequestDeduplicator) cleanup() {
	if len(rd.entries) <= rd.config.MaxCacheSize {
		return
	}

	now := time.Now()
	toDelete := make([]string, 0, len(rd.entries)/2)
	for k, v := range rd.entries {
		if now.After(v.expireAt) {
			toDelete = append(toDelete, k)
		}
		if len(toDelete) >= len(rd.entries)/4 {
			break
		}
	}

	for _, k := range toDelete {
		delete(rd.entries, k)
	}

	if len(rd.entries) > rd.config.MaxCacheSize {
		count := 0
		target := len(rd.entries) / 2
		for k, v := range rd.entries {
			v.mu.Lock()
			if v.done || count < target {
				delete(rd.entries, k)
				count++
			}
			v.mu.Unlock()
			if count >= target {
				break
			}
		}
	}
}

func (rd *RequestDeduplicator) Clear() {
	rd.mu.Lock()
	rd.entries = make(map[string]*dedupEntry)
	rd.mu.Unlock()
}

func cloneRes(r *Res) *Res {
	if r == nil {
		return nil
	}
	cloned := &Res{
		err:             r.err,
		r:               r.r,
		req:             r.req,
		resp:            r.resp,
		client:          r.client,
		multipartHelper: r.multipartHelper,
		cost:            r.cost,
	}
	if r.responseBody != nil {
		cloned.responseBody = make([]byte, len(r.responseBody))
		copy(cloned.responseBody, r.responseBody)
	}
	if r.requesterBody != nil {
		cloned.requesterBody = make([]byte, len(r.requesterBody))
		copy(cloned.requesterBody, r.requesterBody)
	}
	return cloned
}

func (e *Engine) EnableDedup(config ...DedupConfig) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.dedup = NewRequestDeduplicator(config...)
}

func (e *Engine) DisableDedup() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.dedup != nil {
		e.dedup.SetEnabled(false)
	}
}

func (e *Engine) ClearDedup() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.dedup != nil {
		e.dedup.Clear()
	}
}

func EnableDedup(config ...DedupConfig) {
	std.EnableDedup(config...)
}

func DisableDedup() {
	std.DisableDedup()
}

func ClearDedup() {
	std.ClearDedup()
}
