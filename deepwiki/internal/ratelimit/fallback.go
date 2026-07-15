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
}

type entry struct {
	lim      *rate.Limiter
	lastUsed time.Time
}

func newFallbackLimiter() *fallbackLimiter {
	return &fallbackLimiter{
		lims: make(map[string]*entry),
		stop: make(chan struct{}),
	}
}

// allow 按窗口语义近似换算：速率 = limit/window，突发 = limit（滑动窗口上限的保守近似）。
func (f *fallbackLimiter) allow(key string, window time.Duration, limit int) Decision {
	// TODO: 实现兜底判定，要求：
	// ① 不存在则 rate.NewLimiter(rate.Every(window/time.Duration(limit)), limit) 建桶；
	// ② Allow() 判定并更新 lastUsed；Remaining 用 Tokens() 取整近似；
	// ③ 后台 goroutine 每 5 分钟清理 lastUsed 超 10 分钟的 idle 桶（LRU 淘汰，防内存膨胀，硬约束 #4：
	//    goroutine 必须可退出——监听 f.stop）。
	panic("TODO: fallbackLimiter.allow not implemented")
}

func (f *fallbackLimiter) close() { close(f.stop) }
