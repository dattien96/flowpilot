package flowgate

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

// CP-67 P-2b (Task-383): parse-result cache. The gate runs every turn but a
// turn only touches a few files — re-parsing (a node subprocess spawn or an
// LSP round-trip) for unchanged files is pure waste. Keyed by content hash
// (sha256 of the bytes), so an mtime mismatch can never serve stale symbols:
// content decides, the caller just passes what it already read.
//
// Bounded: entries evict FIFO past the cap so a long-lived runner cannot grow
// the map without bound.

const stubBodyCacheCap = 512

type stubBodyCacheEntry struct {
	contentHash string
	symbols     []SymbolInfo
}

// StubBodyCache is safe for concurrent gate evaluations.
type StubBodyCache struct {
	mu    sync.Mutex
	order []string
	m     map[string]stubBodyCacheEntry
}

// NewStubBodyCache returns an empty cache.
func NewStubBodyCache() *StubBodyCache {
	return &StubBodyCache{m: make(map[string]stubBodyCacheEntry)}
}

// ContentHash is the cache key component callers compute from the bytes they
// just read (plus the path — two files with identical content are still
// distinct extraction targets).
func ContentHash(path string, src []byte) string {
	h := sha256.New()
	h.Write([]byte(path))
	h.Write([]byte{0})
	h.Write(src)
	return hex.EncodeToString(h.Sum(nil))
}

// Get returns the cached extraction for (path, src) or nil on miss.
func (c *StubBodyCache) Get(path string, src []byte) []SymbolInfo {
	if c == nil {
		return nil
	}
	key := path + "\x00" + ContentHash(path, src)
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok {
		return nil
	}
	return append([]SymbolInfo(nil), e.symbols...)
}

// Put stores an extraction result.
func (c *StubBodyCache) Put(path string, src []byte, syms []SymbolInfo) {
	if c == nil {
		return
	}
	key := path + "\x00" + ContentHash(path, src)
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.m[key]; !ok {
		c.order = append(c.order, key)
	}
	c.m[key] = stubBodyCacheEntry{contentHash: ContentHash(path, src), symbols: append([]SymbolInfo(nil), syms...)}
	// FIFO eviction past the cap.
	for len(c.order) > stubBodyCacheCap {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.m, oldest)
	}
}

// Len reports the entry count (test use).
func (c *StubBodyCache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.m)
}
