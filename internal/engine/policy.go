// policy.go 每账号调度策略的运行时形态。策略本身存在 SQLite 的
// account_policies 表里（见 store.AccountPolicy），由管理台写入；
// 引擎在进程内持有一份快照，避免选号热路径上查库。
package engine

import "strings"

// PolicyKind 区分策略约束的生成任务类型。
type PolicyKind string

const (
	// PolicyKindImage 图片生成（Flow 的文生图 / Nano Banana）。
	PolicyKindImage PolicyKind = "image"
	// PolicyKindVideo 视频生成（Flow 的文生视频 / Veo）。
	PolicyKindVideo PolicyKind = "video"
)

// AccountPolicy 是账号调度策略的运行时快照。
//
// 这里用「禁止」而不是「允许」描述开关：Go 的 bool 零值是 false，
// 若用「允许」语义，零值就会变成「图片和视频都不接」——与「没有策略」
// 的含义正好相反。改成「禁止」后零值天然表示「不限制」，管理台没配过
// 策略的账号可以直接用零值，行为与策略全开一致。
type AccountPolicy struct {
	// ImageBlocked 该账号被禁止接图片生成任务。
	ImageBlocked bool
	// VideoBlocked 该账号被禁止接视频生成任务。
	VideoBlocked bool
	// ImageConcurrency 图片任务并发上限，0 表示不限。
	ImageConcurrency int
	// VideoConcurrency 视频任务并发上限，0 表示不限。
	VideoConcurrency int
}

// Unlimited 判断策略是否完全不限制该账号（没有禁止项、并发不限）。
// 这样的策略不需要下发到引擎：引擎侧保留既有行为即可。
func (p AccountPolicy) Unlimited() bool {
	return !p.ImageBlocked && !p.VideoBlocked &&
		p.ImageConcurrency <= 0 && p.VideoConcurrency <= 0
}

// EnabledFor 报告该账号是否允许接指定类型的任务。
func (p AccountPolicy) EnabledFor(kind PolicyKind) bool {
	switch kind {
	case PolicyKindImage:
		return !p.ImageBlocked
	case PolicyKindVideo:
		return !p.VideoBlocked
	default:
		// 未识别的任务类型不受图片/视频开关约束。
		return true
	}
}

// ConcurrencyFor 返回该账号对指定类型任务的并发上限；0 表示不限。
func (p AccountPolicy) ConcurrencyFor(kind PolicyKind) int {
	switch kind {
	case PolicyKindImage:
		return p.ImageConcurrency
	case PolicyKindVideo:
		return p.VideoConcurrency
	default:
		return 0
	}
}

// PolicyStore 由能读取账号策略的引擎实现。管理台保存策略后调用
// SetAccountPolicy 把快照推给引擎，引擎据此在选号时过滤与限流。
type PolicyStore interface {
	// SetAccountPolicy 下发（或清除）一个账号的策略快照。
	SetAccountPolicy(accountID string, policy AccountPolicy)
}

// PolicyKindForModel 按模型 ID 推断该模型属于哪一类生成任务。
// 用于把「图片任务」和「视频任务」分别对应到账号策略的两个开关与两个并发上限。
//
// 判定沿用 model.ClassifyKind 的关键词口径（veo/omni → 视频，
// imagen/image/nano-banana → 图片），但这里只做图片/视频的二分：
// 聊天、音乐、语音等任务不受这两个开关约束。
func PolicyKindForModel(modelID string) PolicyKind {
	name := strings.ToLower(strings.TrimSpace(modelID))
	if name == "" {
		return ""
	}
	base := name
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		base = name[idx+1:]
	}
	switch {
	case strings.Contains(base, "veo") || strings.Contains(base, "omni") ||
		strings.Contains(base, "video"):
		return PolicyKindVideo
	case strings.Contains(base, "imagen") || strings.Contains(base, "image") ||
		strings.Contains(base, "nano-banana"):
		return PolicyKindImage
	default:
		return ""
	}
}
