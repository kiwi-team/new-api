package common

import (
	"sync"
	"time"
)

type InMemoryRateLimiter struct {
	store              map[string]*[]int64
	mutex              sync.Mutex
	expirationDuration time.Duration
}

func (l *InMemoryRateLimiter) Init(expirationDuration time.Duration) {
	if l.store == nil {
		l.mutex.Lock()
		if l.store == nil {
			l.store = make(map[string]*[]int64)
			l.expirationDuration = expirationDuration
			if expirationDuration > 0 {
				go l.clearExpiredItems()
			}
		}
		l.mutex.Unlock()
	}
}

func (l *InMemoryRateLimiter) clearExpiredItems() {
	for {
		time.Sleep(l.expirationDuration)
		l.mutex.Lock()
		now := time.Now().Unix()
		for key := range l.store {
			queue := l.store[key]
			size := len(*queue)
			if size == 0 || now-(*queue)[size-1] > int64(l.expirationDuration.Seconds()) {
				delete(l.store, key)
			}
		}
		l.mutex.Unlock()
	}
}

// Request parameter duration's unit is seconds
func (l *InMemoryRateLimiter) Request(key string, maxRequestNum int, duration int64) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	// [old <-- new]
	now := time.Now().Unix()
	cutoff := now - duration

	queue, ok := l.store[key]
	if !ok {
		capHint := maxRequestNum
		if capHint < 1 {
			capHint = 1
		}
		s := make([]int64, 0, capHint)
		s = append(s, now)
		l.store[key] = &s
		return true
	}

	q := *queue

	// 丢弃所有超出时间窗口的旧记录（滑动窗口）
	drop := 0
	for drop < len(q) && q[drop] < cutoff {
		drop++
	}
	if drop > 0 {
		q = q[drop:]
	}

	// 限流配置被调小后，丢弃多余的最早记录，保留最新的 maxRequestNum 条
	if maxRequestNum > 0 && len(q) > maxRequestNum {
		q = q[len(q)-maxRequestNum:]
	}

	if maxRequestNum > 0 && len(q) >= maxRequestNum {
		*queue = q
		return false
	}

	q = append(q, now)
	*queue = q
	return true
}
