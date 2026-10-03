// Package geminiweb 实现 gemini.google.com 网页版的 Go 原生逆向客户端。
// 协议参考 HanaokaYuzu/Gemini-API（MIT），包含：
//   - 初始化：GET /app 提取 SNlM0e（access token）、bl、f.sid 等会话参数
//   - 对话：POST StreamGenerate（f.req JSPB 数组，流式/非流式）
//   - Cookie：本地缓存 + __Secure-1PSIDTS 后台自动轮换
//   - 模型发现：batchexecute 的 GetUserStatus RPC
package geminiweb

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// 端点常量（与 gemini_webapi.constants.Endpoint 一致）。
const (
	EndpointGoogle        = "https://www.google.com"
	EndpointInit          = "https://gemini.google.com/app"
	EndpointGenerate      = "https://gemini.google.com/_/BardChatUi/data/assistant.lamda.BardFrontendService/StreamGenerate"
	EndpointBatchExec     = "https://gemini.google.com/_/BardChatUi/data/batchexecute"
	EndpointRotateCookies = "https://accounts.google.com/RotateCookies"
	DefaultLanguage       = "en"
	DefaultPushID         = "feeds/mcudyrk2a4khkz"
)

// 请求头常量。
const (
	HeaderModelKey       = "x-goog-ext-525001261-jspb"
	HeaderModelStreamKey = "x-goog-ext-525005358-jspb"
	HeaderExtra1         = "x-goog-ext-73010989-jspb"
	HeaderExtra2         = "x-goog-ext-73010990-jspb"
	ContentTypeForm      = "application/x-www-form-urlencoded;charset=utf-8"
)

// 错误码。
const (
	ErrUsageLimitExceeded   = 1037
	ErrModelInconsistent    = 1050
	ErrModelHeaderInvalid   = 1052
	ErrIPTemporarilyBlocked = 1060
)

// 从 init 页面提取的会话参数。
type initParams struct {
	AccessToken string // SNlM0e
	BuildLabel  string // bl
	SessionID   string // f.sid
	Language    string
	PushID      string
}

var (
	reAccessToken = regexp.MustCompile(`"SNlM0e":\s*"(.*?)"`)
	reBuildLabel  = regexp.MustCompile(`"cfb2h":\s*"(.*?)"`)
	reSessionID   = regexp.MustCompile(`"FdrFJe":\s*"(.*?)"`)
	reLanguage    = regexp.MustCompile(`"TuQe5cc":\s*"(.*?)"`)
	rePushID      = regexp.MustCompile(`"qKIAYe":\s*"(.*?)"`)
)

// parseInitParams 从 /app 页面 HTML 提取初始化参数。
func parseInitParams(html string) (*initParams, error) {
	p := &initParams{}
	if m := reAccessToken.FindStringSubmatch(html); len(m) == 2 {
		p.AccessToken = m[1]
	}
	if m := reBuildLabel.FindStringSubmatch(html); len(m) == 2 {
		p.BuildLabel = m[1]
	}
	if m := reSessionID.FindStringSubmatch(html); len(m) == 2 {
		p.SessionID = m[1]
	}
	if m := reLanguage.FindStringSubmatch(html); len(m) == 2 {
		p.Language = m[1]
	}
	if m := rePushID.FindStringSubmatch(html); len(m) == 2 {
		p.PushID = m[1]
	}
	if p.AccessToken == "" && p.BuildLabel == "" && p.SessionID == "" && p.Language == "" && p.PushID == "" {
		return nil, errors.New("init 页面未包含任何初始化参数（Cookie 可能无效）")
	}
	if p.Language == "" {
		p.Language = DefaultLanguage
	}
	if p.PushID == "" {
		p.PushID = DefaultPushID
	}
	return p, nil
}

// defaultMetadata 对应 Python 库的 DEFAULT_METADATA。
var defaultMetadata = []any{"", "", "", nil, nil, nil, nil, nil, nil, ""}

// BuildGenerateForm 构造 StreamGenerate 请求的表单数据。
//
// inner[0]=[prompt,0,nil,fileData,nil,nil,0]，其余字段遵循 gemini-common 的 JSPB 布局：
// [1]=语言 [6]=[1] [7]=1(流式) [10]=1 [11]=0 [17]=[[0]] [18]=0 [27]=1 [30]=[4]
// [41]=[1] [53]=0 [59]=请求UUID [61]=[] [68]=1 [79]=模型号 [80]=思考级别。
func buildGenerateForm(prompt, lang, accessToken string, modelNumber int, extendedThinking, temporary bool) url.Values {
	inner := make([]any, 81)
	inner[0] = []any{prompt, 0, nil, nil, nil, nil, 0}
	inner[1] = []any{lang}
	inner[2] = defaultMetadata
	inner[6] = []any{1}
	inner[7] = 1 // 流式标志
	inner[10] = 1
	inner[11] = 0
	inner[17] = []any{[]any{0}}
	inner[18] = 0
	inner[27] = 1
	inner[30] = []any{4}
	inner[41] = []any{1}
	if temporary {
		inner[45] = 1
	}
	inner[53] = 0
	inner[59] = strings.ToUpper(randID())
	inner[61] = []any{}
	inner[68] = 1
	if modelNumber > 0 {
		inner[79] = modelNumber
	} else {
		inner[79] = 1
	}
	if extendedThinking {
		inner[80] = 2
	} else {
		inner[80] = 1
	}

	innerJSON, _ := json.Marshal(inner)
	// f.req = JSON 数组 [null, "<inner JSON 字符串>"]
	fReq, _ := json.Marshal([]any{nil, string(innerJSON)})

	form := url.Values{}
	form.Set("at", accessToken)
	form.Set("f.req", string(fReq))
	return form
}

// buildModelHeaders 构建模型选择所需的 JSPB 请求头。
// 参考 build_model_header：x-goog-ext-525001261 为 [1,null,null,null,"model_id",null,null,0,[4,5,6,8],null,null,<capacity>, null,null,<model_number>]
func buildModelHeaders(modelID string, capacity, modelNumber int) map[string]string {
	h := map[string]string{
		HeaderModelKey: fmt.Sprintf(`[1,null,null,null,"%s",null,null,0,[4,5,6,8],null,null,%d, null,null,%d]`, modelID, capacity, modelNumber),
		HeaderExtra1:   "[0]",
		HeaderExtra2:   "[0,0,0]",
	}
	return h
}

// buildBatchExecHeaders 构造 batchexecute 的公共请求头。
func buildBatchExecHeaders(sessionID string) map[string]string {
	return map[string]string{
		HeaderModelKey: `[1,null,null,null,null,null,null,null,[4,5,6,8],null,null,null,null,null,null,null]`,
		HeaderExtra1:   "[0]",
	}
}

// rpcPayload batchexecute 的单条 RPC。
type rpcPayload struct {
	RPCID      string
	Data       string
	Identifier string
}

func (r rpcPayload) serialize() []any {
	return []any{r.RPCID, r.Data, nil, r.Identifier}
}

// buildBatchForm 构造 batchexecute 的 form。
func buildBatchForm(accessToken string, payloads []rpcPayload) url.Values {
	list := make([]any, 0, len(payloads))
	for _, p := range payloads {
		list = append(list, p.serialize())
	}
	fReq, _ := json.Marshal([]any{list})
	form := url.Values{}
	form.Set("at", accessToken)
	form.Set("f.req", string(fReq))
	return form
}

// getNested 按索引路径安全读取嵌套值；找不到返回 default。
func getNested(v any, def any, path ...int) any {
	cur := v
	for _, k := range path {
		list, ok := cur.([]any)
		if !ok || k < 0 || k >= len(list) {
			return def
		}
		cur = list[k]
	}
	if cur == nil {
		return def
	}
	return cur
}

// getNestedStr 安全读取字符串。
func getNestedStr(v any, def string, path ...int) string {
	if s, ok := getNested(v, nil, path...).(string); ok {
		return s
	}
	return def
}

// 解析候选数据为文本。
func parseCandidateText(candidate []any) string {
	// 文本在 candidate[1][0]；特殊 card_content 链接在 [22][0]
	text, _ := getNested(candidate, "", 1, 0).(string)
	if strings.HasPrefix(text, "https://googleusercontent.com/card_content/") {
		text, _ = getNested(candidate, "", 22, 0).(string)
	}
	// 清理 googleusercontent 伪影
	text = artifactRe.ReplaceAllString(text, "")
	return text
}

var artifactRe = regexp.MustCompile(`https?://googleusercontent\.com/(?:\w+/)*\d+\n*`)
