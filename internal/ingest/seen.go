package ingest

import (
	"sync"
	"time"
)

// seenCache remembers the IDs of recently ingested events.
//
// The gateway resends a batch when it does not get an answer in time, so the
// same event can be delivered more than once. A redelivered event must not be
// counted again (docs/billing-rules.md §4).
type seenCache struct {
	window time.Duration

	mu        sync.Mutex
	firstSeen map[string]time.Time
	lastSweep time.Time
}

func newSeenCache(window time.Duration) *seenCache {
	return &seenCache{window: window, firstSeen: make(map[string]time.Time)}
}

// SeenOrAdd reports whether id was already ingested within the window. If it
// was not, the id is remembered and SeenOrAdd returns false.
func (c *seenCache) SeenOrAdd(id string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sweep(now)
	if at, ok := c.firstSeen[id]; ok && now.Sub(at) < c.window {
		return true
	}
	c.firstSeen[id] = now
	return false
}

// Forget drops id, so that a later delivery of the same event is accepted.
func (c *seenCache) Forget(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.firstSeen, id)
}

// sweep removes expired entries. It runs at most once per window, which keeps
// the cost per call amortised constant.
func (c *seenCache) sweep(now time.Time) {
	if now.Sub(c.lastSweep) < c.window {
		return
	}
	for id, at := range c.firstSeen {
		if now.Sub(at) >= c.window {
			delete(c.firstSeen, id)
		}
	}
	c.lastSweep = now
}
