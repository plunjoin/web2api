// order.go 管理台配置的账号路由顺序。
//
// 顺序是一个全局的账号 ID 列表：列表里靠前的账号优先接单，同供应商内先走
// 靠前的账号，供应商的先后也由该列表里各自最靠前的账号决定。没有保存过顺序时
// 引擎保持原有轮询行为；保存过之后按列表顺序严格优先，账号失败才换下一个。
package engine

import (
	"sort"
	"strings"
	"sync"
)

// AccountOrderer 由有账号池的引擎实现，接收管理台保存的全局账号顺序。
type AccountOrderer interface {
	SetAccountOrder(ids []string)
}

// AccountOrder 是账号 ID → 优先级的映射，零值可用。
type AccountOrder struct {
	mu    sync.RWMutex
	rank  map[string]int
	order bool
}

// Set 保存顺序；空列表表示恢复原有轮询行为。
func (o *AccountOrder) Set(ids []string) {
	rank := make(map[string]int, len(ids))
	for i, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := rank[id]; !exists {
			rank[id] = i
		}
	}
	o.mu.Lock()
	o.rank = rank
	o.order = len(rank) > 0
	o.mu.Unlock()
}

// Active 是否已配置顺序。
func (o *AccountOrder) Active() bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.order
}

// Rank 返回账号在顺序里的位置；未出现在列表里的账号排到最后。
func (o *AccountOrder) Rank(id string) int {
	o.mu.RLock()
	defer o.mu.RUnlock()
	if !o.order {
		return 0
	}
	if rank, ok := o.rank[id]; ok {
		return rank
	}
	return len(o.rank)
}

// Sort 按配置顺序稳定排序；未配置顺序时保持原样。
// idOf 返回账号的存储 ID（与 accounts 表一致）。
func (o *AccountOrder) Sort[T any](list []T, idOf func(T) string) {
	if !o.Active() || len(list) < 2 {
		return
	}
	sort.SliceStable(list, func(i, j int) bool {
		return o.Rank(idOf(list[i])) < o.Rank(idOf(list[j]))
	})
}
