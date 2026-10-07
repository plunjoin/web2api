//go:build web2api_unit

package geminiweb

import "testing"

func TestStreamingFrameParser(t *testing.T) {
	// 构造一个典型响应：帧前缀 + 长度标记 + JSON
	// 长度按 Python 语义 = 换行符(1) + JSON 字符数；Feed 会展开数组元素
	frame := `[null,"c_1",["r_1"],[4,[]]]` // 27 字符
	body := ")]}'" + "\n" + "28\n" + frame + "\n"

	p := NewStreamingFrameParser()
	frames := p.Feed(body)
	if len(frames) != 4 {
		t.Fatalf("期望展开为 4 个元素，实际 %d: %v", len(frames), frames)
	}
	if frames[1] != "c_1" {
		t.Fatalf("第二个元素应为 c_1: %v", frames[1])
	}
}

func TestStreamingFrameMulti(t *testing.T) {
	// 两帧拼接（长度按 Python 语义 = 换行 1 + JSON 字符数）
	f1 := `["hello"]`
	f2 := `["中文世界🌍"]`
	body := ")]}'" + "\n"
	body += "10\n" + f1 + "\n" // 9 字符 + 1 换行
	body += "11\n" + f2 + "\n" // 9 字符 + 🌍 计 2 单元 + 1 换行 = 11

	p := NewStreamingFrameParser()
	frames := p.Feed(body)
	if len(frames) != 2 {
		t.Fatalf("期望 2 个元素，实际 %d: %v", len(frames), frames)
	}
	if frames[0] != "hello" || frames[1] != "中文世界🌍" {
		t.Fatalf("帧内容错误: %v", frames)
	}
}

func TestParseInitParams(t *testing.T) {
	html := `<html><script>window.WIZ_global_data = {"SNlM0e":"abc123","cfb2h":"bld-1","FdrFJe":"sid-1","TuQe5cc":"zh-CN","qKIAYe":"push-1"};</script></html>`
	p, err := parseInitParams(html)
	if err != nil {
		t.Fatal(err)
	}
	if p.AccessToken != "abc123" || p.BuildLabel != "bld-1" || p.SessionID != "sid-1" || p.Language != "zh-CN" || p.PushID != "push-1" {
		t.Fatalf("解析结果不符: %+v", p)
	}
}

func TestParseInitParamsEmpty(t *testing.T) {
	if _, err := parseInitParams(`<html>no tokens</html>`); err == nil {
		t.Fatal("期望空页面报错")
	}
}

func TestBuildGenerateForm(t *testing.T) {
	form := buildGenerateForm("你好", "en", "token123", 1, false, true)
	if form.Get("at") != "token123" {
		t.Fatalf("at 参数错误: %q", form.Get("at"))
	}
	freq := form.Get("f.req")
	if freq == "" {
		t.Fatal("f.req 为空")
	}
	var outer []any
	if err := jsonUnmarshal([]byte(freq), &outer); err != nil {
		t.Fatalf("f.req 不是合法 JSON: %v", err)
	}
	if len(outer) != 2 || outer[0] != nil {
		t.Fatalf("f.req 结构错误: %v", outer)
	}
	innerStr, ok := outer[1].(string)
	if !ok {
		t.Fatalf("f.req[1] 应为字符串: %T", outer[1])
	}
	var inner []any
	if err := jsonUnmarshal([]byte(innerStr), &inner); err != nil {
		t.Fatalf("内层 JSON 不合法: %v", err)
	}
	if len(inner) != 81 {
		t.Fatalf("内层数组长度应为 81: %d", len(inner))
	}
	p0, ok := inner[0].([]any)
	if !ok || len(p0) == 0 || p0[0] != "你好" {
		t.Fatalf("inner[0] 错误: %v", inner[0])
	}
	// [7] 流式标志（JSON 数字为 float64）
	if inner[7] != float64(1) {
		t.Fatalf("inner[7] 应为 1: %v", inner[7])
	}
	// [45] 临时会话标志
	if inner[45] != float64(1) {
		t.Fatalf("inner[45] 应为 1: %v", inner[45])
	}
}

func TestBuildModelHeaders(t *testing.T) {
	h := buildModelHeaders("fbb127bbb056c959", 1, 1)
	want := `[1,null,null,null,"fbb127bbb056c959",null,null,0,[4,5,6,8],null,null,1, null,null,1]`
	if h[HeaderModelKey] != want {
		t.Fatalf("模型头错误:\n got %s\nwant %s", h[HeaderModelKey], want)
	}
}

func TestParseCandidateText(t *testing.T) {
	// 标准文本候选
	cand := []any{
		"rcid-1",
		[]any{"回答内容"},
	}
	got := parseCandidateText(cand)
	if got != "回答内容" {
		t.Fatalf("候选解析错误: %q", got)
	}

	// card_content 场景
	cand2 := make([]any, 23)
	cand2[0] = "rcid-2"
	cand2[1] = []any{"https://googleusercontent.com/card_content/123"}
	cand2[22] = []any{"真实内容"}
	got2 := parseCandidateText(cand2)
	if got2 != "真实内容" {
		t.Fatalf("card_content 解析错误: %q", got2)
	}
}

func TestParseModelsFromRPC(t *testing.T) {
	// 构造 GetUserStatus 响应帧：part[2] 为内层 JSON 字符串
	// JSPB 稀疏数组：位置 14=status, 15=模型列表, 16=tier_flags, 17=capability_flags
	inner := `[null,null,null,null,null,null,null,null,null,null,null,null,null,null,1000,[["model-abc","Flash","Gemini 3 Flash","desc"]],[8],[19]]`
	part := []any{
		nil,
		"otAQ7b",
		inner,
	}
	frames := []any{part}
	models := parseModelsFromRPC(frames)
	if len(models) == 0 {
		t.Fatal("期望解析出模型")
	}
	m := models[0]
	if m.ID != "model-abc" {
		t.Fatalf("模型ID错误: %q", m.ID)
	}
	if m.Name != "gemini-flash" {
		t.Fatalf("模型名错误: %q", m.Name)
	}
	if m.Capacity != 2 { // tier 含 8 → Pro capacity 2
		t.Fatalf("capacity 错误: %d", m.Capacity)
	}
	if m.ModelNumber != 1 {
		t.Fatalf("model number 错误: %d", m.ModelNumber)
	}
}
