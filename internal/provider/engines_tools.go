package provider

import (
	"web2api/internal/engine"
	"web2api/internal/engine/aistudio"
	"web2api/internal/engine/antigravity"
	"web2api/internal/engine/codex"
	"web2api/internal/engine/custom"
	"web2api/internal/engine/workbuddy"
)

// 编译期断言：支持 function calling 的引擎必须实现 engine.ToolEngine。
//
// 网关按接口断言选择路径（见 Manager.ChatStream）。曾经断言失败不会报错、
// 只会静默退化成「忽略 tools、仅回文本」，调用方完全无从察觉工具被吞，
// 这种退化在人工测试里极难发现。现在 Manager.requireToolSupport 会在请求
// 带 tools 但引擎不支持时显式返回 engine.ToolsNotSupportedError，不再静默。
var (
	_ engine.ToolEngine = (*custom.Engine)(nil)
	_ engine.ToolEngine = (*aistudio.Engine)(nil)
	_ engine.ToolEngine = (*aistudio.NativeEngine)(nil)
	_ engine.ToolEngine = (*codex.Engine)(nil)
	_ engine.ToolEngine = (*antigravity.Engine)(nil)
	_ engine.ToolEngine = (*workbuddy.Engine)(nil)
)
