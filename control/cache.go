package control

import (
	"strings"
	"sync"

	"github.com/golang/groupcache/singleflight"
	"tier.run/lru"
)

type orgKey struct {
	account string
	clock   string // a test clock, if any
	name    string
}

type memo struct {
	m     sync.Mutex
	lru   *lru.Cache[orgKey, string] // map[orgKey] -> customerID
	group singleflight.Group
}

func (m *memo) lookupCache(key orgKey) (string, bool) {
	m.m.Lock()
	defer m.m.Unlock()
	if m.lru == nil {
		return "", false
	}
	return m.lru.Get(key)
}

func (m *memo) load(key orgKey, fn func() (string, error)) (string, error) {
	s, cacheHit := m.lookupCache(key)
	if cacheHit {
		return s, nil
	}

	// TODO(bmizerany): make a singleflight with generics to avoid building
	// a string instead of using orgKey as a key
	var b strings.Builder
	b.WriteString(key.account)
	b.WriteString(key.clock)
	b.WriteString(key.name)

	v, err := m.group.Do(b.String(), func() (any, error) {
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

func (m *memo) add(key orgKey, val string) {
	m.m.Lock()
	defer m.m.Unlock()
	if m.lru == nil {
		m.lru = lru.New[orgKey, string](100) // TODO(bmizerany): make configurable
	}
	m.lru.Add(key, val)
}
