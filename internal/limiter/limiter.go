// Package limiter 提供简单的令牌桶限流（按 Key 维度，如 API Key 或 IP）。
package limiter

import (
	"sync"
	"time"
)

// Bucket 单桶。
type Bucket struct {
	mu       sync.Mutex
	tokens   float64
	capacity float64
	rate     float64 // 每秒补充
	last     time.Time
}

// Limiter 限流器。
type Limiter struct {
	mu       sync.Mutex
	buckets  map[string]*Bucket
	rate     float64
	capacity float64
}

// New 创建限流器。rate<=0 表示不限流。
func New(rate float64, capacity float64) *Limiter {
	if rate <= 0 {
		return &Limiter{rate: 0}
	}
	// 至少保留一个完整令牌，否则 capacity=0 或小于 1 时永远不会
	// 允许请求；省略容量时按每秒速率给出合理的突发额度。
	if capacity <= 0 {
		capacity = rate
	}
	if capacity < 1 {
		capacity = 1
	}
	return &Limiter{
		buckets:  map[string]*Bucket{},
		rate:     rate,
		capacity: capacity,
	}
}

// Allow 检查 key 是否允许请求。
func (l *Limiter) Allow(key string) bool {
	if l.rate <= 0 {
		return true
	}
	l.mu.Lock()
	b, ok := l.buckets[key]
	if !ok {
		b = &Bucket{
			tokens:   l.capacity,
			rate:     l.rate,
			capacity: l.capacity,
			last:     time.Now(),
		}
		l.buckets[key] = b
	}
	l.mu.Unlock()

	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(b.last).Seconds()
	b.last = now
	b.tokens += elapsed * b.rate
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}
