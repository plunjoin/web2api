// affinity.go 会话粘性所需的请求级账号标记。
//
// 网关把一个会话键映射到上次真正接单的账号，下一次请求通过
// WithPreferredAccount 指定它；引擎在候选列表里把它排到最前（仍可故障切换），
// 成功后再用 NoteServedAccount 回传实际接单账号，供网关刷新映射。
// 跟踪状态只存在于本次请求的 context 中，不改变引擎的全局顺序。
package engine

import (
	"context"
	"strings"
	"sync"
)

type affinityState struct {
	mu           sync.Mutex
	preferred    string
	served       string
	conversation string
}

type affinityStateKey struct{}

// WithAccountTracking 为本次请求开启「实际接单账号」回传。网关在调用引擎前
// 总是先调用它；引擎成功接单后调用 NoteServedAccount。
func WithAccountTracking(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Value(affinityStateKey{}).(*affinityState); ok {
		return ctx
	}
	return context.WithValue(ctx, affinityStateKey{}, &affinityState{})
}

// WithPreferredAccount 指定本次请求优先尝试的账号（存储 ID）。
// 账号不可用时引擎照常尝试其他账号，保证粘性只是偏好，不是硬绑定。
func WithPreferredAccount(ctx context.Context, id string) context.Context {
	id = strings.TrimSpace(id)
	ctx = WithAccountTracking(ctx)
	if id == "" {
		return ctx
	}
	if st, ok := ctx.Value(affinityStateKey{}).(*affinityState); ok {
		st.mu.Lock()
		st.preferred = id
		st.mu.Unlock()
	}
	return ctx
}

// PreferredAccount 返回本次请求优先使用的账号 ID；没有时返回空串。
func PreferredAccount(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	st, ok := ctx.Value(affinityStateKey{}).(*affinityState)
	if !ok || st == nil {
		return ""
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.preferred
}

// NoteServedAccount 由引擎在某个账号成功接单后调用；多次调用以最后一次为准。
func NoteServedAccount(ctx context.Context, id string) {
	if ctx == nil || strings.TrimSpace(id) == "" {
		return
	}
	st, ok := ctx.Value(affinityStateKey{}).(*affinityState)
	if !ok || st == nil {
		return
	}
	st.mu.Lock()
	st.served = id
	st.mu.Unlock()
}

// ServedAccount 返回本次请求实际接单的账号；引擎未上报时返回空。
func ServedAccount(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	st, ok := ctx.Value(affinityStateKey{}).(*affinityState)
	if !ok || st == nil {
		return "", false
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.served, st.served != ""
}

// WithConversationID 为本次请求绑定稳定的会话 ID。引擎可把它透传给上游
// （WorkBuddy 的 X-Conversation-ID 等），让同一会话的请求命中上游前缀缓存。
func WithConversationID(ctx context.Context, id string) context.Context {
	id = strings.TrimSpace(id)
	if id == "" {
		return ctx
	}
	ctx = WithAccountTracking(ctx)
	if st, ok := ctx.Value(affinityStateKey{}).(*affinityState); ok {
		st.mu.Lock()
		st.conversation = id
		st.mu.Unlock()
	}
	return ctx
}

// ConversationID 返回本次请求的会话 ID；未绑定时返回空。
func ConversationID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	st, ok := ctx.Value(affinityStateKey{}).(*affinityState)
	if !ok || st == nil {
		return ""
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.conversation
}

// PreferAccount 把列表里匹配 preferred 的账号稳定地移到最前，其余顺序不变。
// 未指定偏好或列表里没有该账号时原样返回。
func PreferAccount[T any](ctx context.Context, list []T, idOf func(T) string) []T {
	id := PreferredAccount(ctx)
	if id == "" || len(list) < 2 {
		return list
	}
	for i, item := range list {
		if idOf(item) != id {
			continue
		}
		if i == 0 {
			return list
		}
		preferred := list[i]
		copy(list[1:i+1], list[:i])
		list[0] = preferred
		return list
	}
	return list
}
