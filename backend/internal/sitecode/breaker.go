package sitecode

import (
	"container/list"
	"errors"
	"sync"
	"time"
)

// Breaker disables a handler that keeps failing: Threshold failures within
// Window open it for Cooldown.
type Breaker struct {
	Threshold int
	Window    time.Duration
	Cooldown  time.Duration
	Now       func() time.Time // tests

	mu    sync.Mutex
	state map[string]*breakerState
}

type breakerState struct {
	fails     []time.Time
	openUntil time.Time
}

// NewBreaker returns the default breaker: 20 failures / 1 minute → 5 minutes off.
func NewBreaker() *Breaker {
	return &Breaker{Threshold: 20, Window: time.Minute, Cooldown: 5 * time.Minute, Now: time.Now,
		state: map[string]*breakerState{}}
}

// Allow reports whether key may run now.
func (b *Breaker) Allow(key string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.state[key]
	return s == nil || !b.Now().Before(s.openUntil)
}

// Fail records a failure; true when this failure opened the breaker.
func (b *Breaker) Fail(key string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.Now()
	s := b.state[key]
	if s == nil {
		s = &breakerState{}
		b.state[key] = s
	}
	keep := s.fails[:0]
	for _, t := range s.fails {
		if now.Sub(t) < b.Window {
			keep = append(keep, t)
		}
	}
	s.fails = append(keep, now)
	if len(s.fails) >= b.Threshold {
		s.fails, s.openUntil = nil, now.Add(b.Cooldown)
		return true
	}
	return false
}

// Reset forgets every failure (a new version went live).
func (b *Breaker) Reset() {
	b.mu.Lock()
	b.state = map[string]*breakerState{}
	b.mu.Unlock()
}

// ---------------------------------------------------------------- cache

// cacheCap bounds the in-memory cache scripts share (JSON bytes + keys).
const cacheCap = 10 << 20

var errCacheTooBig = errors.New("ค่าที่จะเก็บใน cache ใหญ่เกินไป")

// Cache is the per-site in-memory key/value store of cache.get/set/delete:
// values are JSON text, evicted oldest-first beyond the cap.
type Cache struct {
	Cap int
	Now func() time.Time

	mu    sync.Mutex
	items map[string]*list.Element
	order *list.List // front = oldest write
	size  int
}

type cacheItem struct {
	key     string
	value   string
	expires time.Time // zero = never
}

// NewCache returns an empty cache with the default cap.
func NewCache() *Cache {
	return &Cache{Cap: cacheCap, Now: time.Now, items: map[string]*list.Element{}, order: list.New()}
}

// Get returns the JSON text stored under key.
func (c *Cache) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el := c.items[key]
	if el == nil {
		return "", false
	}
	it := el.Value.(*cacheItem)
	if !it.expires.IsZero() && !c.Now().Before(it.expires) {
		c.remove(el)
		return "", false
	}
	return it.value, true
}

// Set stores JSON text for ttl (0 = no expiry).
func (c *Cache) Set(key, value string, ttl time.Duration) error {
	n := len(key) + len(value)
	if n > c.Cap {
		return errCacheTooBig
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el := c.items[key]; el != nil {
		c.remove(el)
	}
	// drop expired entries, then the oldest, until the new value fits
	if c.size+n > c.Cap {
		now := c.Now()
		for el := c.order.Front(); el != nil; {
			next := el.Next()
			if it := el.Value.(*cacheItem); !it.expires.IsZero() && !now.Before(it.expires) {
				c.remove(el)
			}
			el = next
		}
	}
	for c.size+n > c.Cap && c.order.Len() > 0 {
		c.remove(c.order.Front())
	}
	it := &cacheItem{key: key, value: value}
	if ttl > 0 {
		it.expires = c.Now().Add(ttl)
	}
	c.items[key] = c.order.PushBack(it)
	c.size += n
	return nil
}

// Delete removes key.
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el := c.items[key]; el != nil {
		c.remove(el)
	}
}

func (c *Cache) remove(el *list.Element) {
	it := el.Value.(*cacheItem)
	c.order.Remove(el)
	delete(c.items, it.key)
	c.size -= len(it.key) + len(it.value)
}
