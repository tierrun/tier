// Package lru implements a LRU cache. It is a generic vesion of
// github.com/golang/groupcache/lru.
package lru

import "container/list"

// Cache is an LRU cache. It is not safe for concurrent access.
type Cache[K comparable, V any] struct {
	// MaxEntries is the maximum number of cache entries before
	// an item is evicted. Zero means no limit.
	MaxEntries int

	// OnEvicted optionally specifies a callback function to be
	// executed when an entry is purged from the cache.
	OnEvicted func(key K, value V)

	ll    *list.List
	cache map[K]*list.Element
}

type entry[K comparable, V any] struct {
	key   K
	value V
}

// New creates a new Cache.
// If maxEntries is zero, the cache has no limit and it's assumed
// that eviction is done by the caller.
func New[K comparable, V any](maxEntries int) *Cache[K, V] {
	return &Cache[K, V]{
		MaxEntries: maxEntries,
		ll:         list.New(),
		cache:      make(map[K]*list.Element),
	}
}

// Add adds a value to the cache.
func (c *Cache[K, V]) Add(key K, value V) {
	if c.cache == nil {
		c.cache = make(map[K]*list.Element)
		c.ll = list.New()
	}
	if ee, ok := c.cache[key]; ok {
		c.ll.MoveToFront(ee)
		ee.Value.(*entry[K, V]).value = value
		return
	}
	ele := c.ll.PushFront(&entry[K, V]{key, value})
	c.cache[key] = ele
	if c.MaxEntries != 0 && c.ll.Len() > c.MaxEntries {
		c.RemoveOldest()
	}
}

// Get looks up a key's value from the cache.
func (c *Cache[K, V]) Get(key K) (value V, ok bool) {
	if c.cache == nil {
		return
	}
	if ele, hit := c.cache[key]; hit {
		c.ll.MoveToFront(ele)
		return ele.Value.(*entry[K, V]).value, true
	}
	return
}

// Remove removes the provided key from the cache.
func (c *Cache[K, V]) Remove(key K) {
	if c.cache == nil {
		return
	}
	if ele, hit := c.cache[key]; hit {
		c.removeElement(ele)
	}
}

// RemoveOldest removes the oldest item from the cache.
func (c *Cache[K, V]) RemoveOldest() {
	if c.cache == nil {
		return
	}
	ele := c.ll.Back()
	if ele != nil {
		c.removeElement(ele)
	}
}

func (c *Cache[K, V]) removeElement(e *list.Element) {
	c.ll.Remove(e)
	kv := e.Value.(*entry[K, V])
	delete(c.cache, kv.key)
	if c.OnEvicted != nil {
		c.OnEvicted(kv.key, kv.value)
	}
}

// Len returns the number of items in the cache.
func (c *Cache[K, V]) Len() int {
	if c.cache == nil {
		return 0
	}
	return c.ll.Len()
}

// Clear purges all stored items from the cache.
func (c *Cache[K, V]) Clear() {
	if c.OnEvicted != nil {
		for _, e := range c.cache {
			kv := e.Value.(*entry[K, V])
			c.OnEvicted(kv.key, kv.value)
		}
	}
	c.ll = nil
	c.cache = nil
}
