// accounts.go 各引擎共享的账号实时状态结构（供管理台展示）。
package engine

// AccountCooldown 账号仍在生效的额度冷却。Until 为 Unix 秒。
type AccountCooldown struct {
	Scope  string `json:"scope"`
	Until  int64  `json:"until"`
	Reason string `json:"reason,omitempty"`
}

// QuotaWindow 是可计算的额度窗口。只在上游返回绝对额度时提供 Limit/Remaining，
// 未知额度不填窗口，不通过解析展示文案或百分比推算额度。
type QuotaWindow struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Unit      string  `json:"unit"`
	Limit     float64 `json:"limit"`
	Remaining float64 `json:"remaining"`
	ResetsAt  string  `json:"resets_at,omitempty"`
	ExpiresAt string  `json:"expires_at,omitempty"`
	Expiring  float64 `json:"expiring,omitempty"`
	Aside     bool    `json:"aside,omitempty"` // 分项展示，不与总计重复相加
}

// QuotaSnapshot 是观测快照，不直接参与选号；Error/Stale 不等于额度为零。
type QuotaSnapshot struct {
	ReadAt  int64         `json:"read_at"`
	Error   string        `json:"error,omitempty"`
	Stale   bool          `json:"stale,omitempty"`
	Windows []QuotaWindow `json:"windows,omitempty"`
}

// AccountState 账号实时状态（供管理台）。
type AccountState struct {
	ID            string            `json:"id"`
	Label         string            `json:"label"`
	Enabled       bool              `json:"enabled"`
	Ready         bool              `json:"ready"`
	Status        string            `json:"status"`
	Detail        string            `json:"detail,omitempty"`
	Models        int               `json:"models"`
	Tier          string            `json:"tier,omitempty"`
	Quota         string            `json:"quota,omitempty"`   // 用量/余额描述（订阅引擎）
	Service       string            `json:"service,omitempty"` // 同引擎内的独立发行版
	QuotaSnapshot *QuotaSnapshot    `json:"quota_snapshot,omitempty"`
	Cooldowns     []AccountCooldown `json:"cooldowns,omitempty"`
}
