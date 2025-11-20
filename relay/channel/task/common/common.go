package task

import "sync"

type BoundedCache struct {
	mu       sync.Mutex
	data     map[string]string
	order    []string
	capacity int
}

func NewBoundedCache(cap int) *BoundedCache {
	return &BoundedCache{
		data:     make(map[string]string),
		order:    make([]string, 0, cap),
		capacity: cap,
	}
}

func (c *BoundedCache) Get(k string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.data[k]
	return v, ok
}

func (c *BoundedCache) Set(k, v string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.data[k]; !exists {
		c.order = append(c.order, k)
	}
	c.data[k] = v
	if len(c.order) > c.capacity {
		old := c.order[0]
		c.order = c.order[1:]
		delete(c.data, old)
	}
}

var FinishedTaskCache = NewBoundedCache(1000)
