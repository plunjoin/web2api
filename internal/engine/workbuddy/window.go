// window.go 统计 deepseek-v4.1-flash 的专属 token 窗口，复刻 magpie 插件的
// CN_ESTIMATED_WINDOWS / freeWindowOf。
//
// WorkBuddy 对 deepseek-v4.1-flash 有一个独立的 token 配额（按账号、按 24h），
// 积分接口（get-user-resource-summary）看不到它，只有 429 拒绝时在文案里出现
// （"将在 … UTC+8 重置" / "reset at …"）。cap 是实测值：国服约 2e8，国际服约
// 1e8。窗口是固定 24h，从上一个窗口结束后第一条被应答的请求开始算。
//
// 读法有两种，拒绝优先（与 magpie 一致）：
//   - 本进程记录的该账号 deepseek 请求 token 之和（in + out，见下）——直到账号
//     第一次被拒绝前都成立，是估算，展示用（Aside，不参与停号）。
//   - 拒绝文案里带的重置时刻仍在前方——这是唯一的精确信号，把卡片钉满。
//
// 注意：WorkBuddy 上游的 usage.prompt_tokens 已含缓存 token（OpenAI 兼容语义），
// 所以这里用 prompt_tokens + completion_tokens 即等价于 magpie 的
// in + cache_read + out。
package workbuddy

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	dsSpan       = 24 * time.Hour
	dsFlashID    = "deepseek-v4.1-flash"
	dsFlashAlias = "deepseek-v4.1-flash-sg"
)

// dsModelName 报告模型是否落在 deepseek-v4.1-flash 的窗口里（含 -sg 别名）。
func dsModelName(model string) bool {
	model = strings.TrimSpace(model)
	return model == dsFlashID || model == dsFlashAlias || strings.HasPrefix(model, dsFlashID+"@")
}

// dsRow 是本账号应答过的一条 deepseek 请求。
type dsRow struct {
	at     time.Time
	tokens float64
}

// dsWindowSpec 是某个发行版的窗口口径。
type dsWindowSpec struct {
	cap   float64
	name  string
	short string
	unit  string
}

func dsSpec(site Site) dsWindowSpec {
	if site.ID == SiteIntl.ID {
		return dsWindowSpec{cap: 1e8, name: "Free model tokens", short: "100M", unit: "tokens"}
	}
	return dsWindowSpec{cap: 2e8, name: "deepseek-v4.1-flash tokens (est.)", short: "~200M"}
}

var (
	dsResetRe  = regexp.MustCompile(`(?:reset at|将在)\s*(\d{4})-(\d{2})-(\d{2})[ T](\d{2}):(\d{2}):(\d{2})`)
	dsOffsetRe = regexp.MustCompile(`(?:UTC|GMT)\s*([+-])\s*(\d{1,2})(?::?(\d{2}))?`)
)

// windowReset 从拒绝文案里解析重置时刻（国服"将在 … 重置"、国际服"reset at …"），
// 带 UTC±偏移；偏移读不出返回零值（偏移猜错会差几小时，宁可不用于卡边）。
func windowReset(text string) time.Time {
	m := dsResetRe.FindStringSubmatch(text)
	if m == nil {
		return time.Time{}
	}
	om := dsOffsetRe.FindStringSubmatch(text)
	if om == nil {
		return time.Time{}
	}
	off := 0
	if v, err := strconv.Atoi(om[2]); err == nil {
		off = v * 60
	}
	if v, err := strconv.Atoi(om[3]); err == nil {
		off += v
	}
	if om[1] == "-" {
		off = -off
	}
	y, _ := strconv.Atoi(m[1])
	mo, _ := strconv.Atoi(m[2])
	d, _ := strconv.Atoi(m[3])
	h, _ := strconv.Atoi(m[4])
	mi, _ := strconv.Atoi(m[5])
	s, _ := strconv.Atoi(m[6])
	return time.Date(y, time.Month(mo), d, h, mi, s, 0, time.UTC).Add(-time.Duration(off) * time.Minute)
}

// dsWindow 从本账号的 deepseek 请求记录算出当前窗口卡片；没有窗口返回 nil。
// restUntil 是仍在生效的拒绝重置时刻（零值表示没有）；now 传入便于测试。
func dsWindow(rows []dsRow, restUntil time.Time, now time.Time, spec dsWindowSpec) *Window {
	// 拒绝优先：仍在生效的拒绝是精确信号，卡片直接钉满到上游给的重置时刻。
	if !restUntil.IsZero() && restUntil.After(now) {
		return dsCard(spec.cap, restUntil, spec, true)
	}

	answered := make([]dsRow, 0, len(rows))
	for _, r := range rows {
		if !r.at.IsZero() && r.tokens > 0 {
			answered = append(answered, r)
		}
	}
	if len(answered) == 0 {
		return nil
	}
	sort.Slice(answered, func(i, j int) bool { return answered[i].at.Before(answered[j].at) })

	// head 从第 from 行起算一段窗口的起点与 token 数。窗口在两条间隔 ≥24h 的
	// 请求之间断开（前一条的窗口到它之后 24h 就结束，所以后一条开新窗）。
	head := func(from int) (time.Time, float64) {
		var start time.Time
		var used float64
		for i := from; i < len(answered); i++ {
			t := answered[i].at
			if start.IsZero() {
				start = t
			} else if t.Sub(start) >= dsSpan {
				start = t
				used = 0
			}
			used += answered[i].tokens
		}
		return start, used
	}

	// 只从「可能开新窗口」的行起算：前一行距它 ≥24h 才可能是链头。
	var start time.Time
	var used float64
	for i := range answered {
		if i > 0 && answered[i].at.Sub(answered[i-1].at) < dsSpan {
			continue
		}
		s, u := head(i)
		if s.IsZero() {
			continue
		}
		if !s.Add(dsSpan).After(now) {
			continue // 该窗口已过时，下一条请求开新窗
		}
		start, used = s, u
		break
	}
	if start.IsZero() {
		return nil
	}
	return dsCard(used, start.Add(dsSpan), spec, false)
}

// dsCard 把窗口压成展示卡片。Aside 恒为 true：Exhausted 是账号级的，不能因为
// 一个模型被限流就停掉整个账号；deepseek 的模型级停号由 chatLoop 的 restUntil
// 在选号时处理。
func dsCard(amount float64, resetsAt time.Time, spec dsWindowSpec, refused bool) *Window {
	if refused {
		amount = spec.cap
	}
	w := &Window{
		Name:     spec.name,
		Amount:   amount,
		Limit:    spec.cap,
		Unit:     spec.unit,
		Display:  tokensShort(amount) + " / " + spec.short,
		ResetsAt: resetsAt.UTC().Format(time.RFC3339),
		Aside:    true,
	}
	if spec.cap > 0 {
		w.Used = 100 * amount / spec.cap
	}
	return w
}

// tokensShort 把 token 数压短：11.9M、812k、128。积分卡片的 compact 读 token 数会很难看。
func tokensShort(n float64) string {
	if n >= 1e6 {
		return fmt.Sprintf("%.1fM", n/1e6)
	}
	if n >= 1e3 {
		return fmt.Sprintf("%dk", int(math.Round(n/1e3)))
	}
	return fmt.Sprintf("%.0f", n)
}
