package tier

import (
	"sync"

	"github.com/golang/groupcache/lru"
	"github.com/golang/groupcache/singleflight"
)

type memo struct {
	m     sync.Mutex
	lru   *lru.Cache
	group singleflight.Group
}

func (m *memo) lookupCache(key string) (string, bool) {
	m.m.Lock()
	defer m.m.Unlock()
	if m.lru == nil {
		return "", false
	}
	v, ok := m.lru.Get(key)
	if !ok {
		return "", false
	}
	return v.(string), true
}

func (m *memo) load(key string, fn func() (string, error)) (string, error) {
	s, cacheHit := m.lookupCache(key)
	if cacheHit {
		return s, nil
	}

	v, err := m.group.Do(key, func() (any, error) {
		v, cacheHit := m.lookupCache(key)
		if cacheHit {
			return v, nil
		}
		s, err := fn()
		if err != nil {
			return "", err
		}
		m.add(key, s)
		return s, nil
	})
	if err != nil {
		return "", err
	}

	return v.(string), nil
}

func (m *memo) add(key, val string) {
	m.m.Lock()
	defer m.m.Unlock()
	if m.lru == nil {
		m.lru = lru.New(100) // TODO(bmizerany): make configurable
	}
	m.lru.Add(key, val)
}
