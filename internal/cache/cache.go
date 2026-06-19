// Package cache provides a small, type-safe TTL cache used to be polite to the
// upstream City of Cape Town service and to speed up repeated queries.
//
// A nil *Cache, or one constructed with a non-positive TTL, is "disabled": every
// [Fetch] simply invokes the loader. This lets callers treat caching as an
// always-available, zero-configuration concern.
package cache

import (
	"time"

	"github.com/jellydator/ttlcache/v3"
)

// Cache is a string-keyed TTL cache storing arbitrary values.
type Cache struct {
	inner *ttlcache.Cache[string, any]
}

// New returns a Cache with the given TTL and optional capacity (0 = unbounded).
// A non-positive ttl returns a disabled cache that never stores anything.
func New(ttl time.Duration, capacity uint64) *Cache {
	if ttl <= 0 {
		return &Cache{}
	}
	opts := []ttlcache.Option[string, any]{ttlcache.WithTTL[string, any](ttl)}
	if capacity > 0 {
		opts = append(opts, ttlcache.WithCapacity[string, any](capacity))
	}
	inner := ttlcache.New[string, any](opts...)
	go inner.Start()
	return &Cache{inner: inner}
}

// Stop releases the cache's background eviction goroutine. Safe on a nil or
// disabled Cache.
func (c *Cache) Stop() {
	if c != nil && c.inner != nil {
		c.inner.Stop()
	}
}

// Fetch returns the cached value for key, or calls load and stores its result.
// The loader is never invoked on a cache hit. Errors are not cached.
func Fetch[V any](c *Cache, key string, load func() (V, error)) (V, error) {
	if c == nil || c.inner == nil {
		return load()
	}
	if item := c.inner.Get(key); item != nil {
		if v, ok := item.Value().(V); ok {
			return v, nil
		}
	}
	v, err := load()
	if err != nil {
		var zero V
		return zero, err
	}
	c.inner.Set(key, v, ttlcache.DefaultTTL)
	return v, nil
}
