package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// fallbackLimiter 进程内 x/time/rate 降级兜底（总纲 §4.4：Redis 不可用时启用 + WARN +
// health degraded；多副本下各节点独立计数是已知近似，可用性优先的有意取舍）。
type fallbackLimiter struct {
	mu   sync.Mutex
	lims map[string]*entry
	stop chan struct{}
	once sync.Once
}

type entry struct {
	lim      *rate.Limiter
	lastUsed time.Time
}

func newFallbackLimiter() *fallbackLimiter {
	f := &fallbackLimiter{
		lims: make(map[string]*entry),
		stop: make(chan struct{}),
	}
	go f.janitor()
	return f
}

// allow 按窗口语义近似换算：速率 = limit/window，突发 = limit（滑动窗口上限的保守近似）。
func (f *fallbackLimiter) allow(key string, window time.Duration, limit int) Decision {
	f.mu.Lock()
	e, ok := f.lims[key]
	if !ok {
		r := rate.Every(window / time.Duration(limit))
		e = &entry{lim: rate.NewLimiter(r, limit), lastUsed: time.Now()}
		f.lims[key] = e
	}
	e.lastUsed = time.Now()
	lim := e.lim
	f.mu.Unlock()

	allowed := lim.Allow()
	remaining := int(lim.Tokens())
	if remaining < 0 {
		remaining = 0
	}
	if remaining > limit {
		remaining = limit
	}

	d := Decision{
		Allowed:   allowed,
		Limit:     limit,
		Remaining: remaining,
		ResetUnix: time.Now().Add(window).Unix(),
		Degraded:  true,
	}
	if !allowed {
		// 近似：一个令牌的恢复时间。
		d.RetryAfter = window / time.Duration(limit)
	}
	return d
}

// janitor 每 5 分钟清理 lastUsed 超 10 分钟的 idle 桶（防内存膨胀；goroutine 必须可退出）。
func (f *fallbackLimiter) janitor() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-f.stop:
			return
		case <-ticker.C:
			cutoff := time.Now().Add(-10 * time.Minute)
			f.mu.Lock()
			for k, e := range f.lims {
				if e.lastUsed.Before(cutoff) {
					delete(f.lims, k)
				}
			}
			f.mu.Unlock()
		}
	}
}

func (f *fallbackLimiter) close() {
	f.once.Do(func() { close(f.stop) })
}
