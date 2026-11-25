package cdp

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLaunchAndEvaluate 冒烟测试：启动本机浏览器、开个页面、执行一段 JS。
// 没有可用浏览器时跳过（CI/容器里通常没有 Chrome）。
func TestLaunchAndEvaluate(t *testing.T) {
	if os.Getenv("WEB2API_FLOW_TEST_BROWSER") == "" && FindBrowser() == "" {
		t.Skip("本机没有 Chrome / Edge，跳过 CDP 冒烟测试")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	browser, err := Launch(ctx, LaunchOptions{Headless: true, Timeout: 40 * time.Second})
	if err != nil {
		t.Fatalf("启动浏览器失败: %v", err)
	}
	defer browser.Close()

	browserContextID, err := browser.NewContext(ctx)
	if err != nil {
		t.Fatalf("创建浏览器上下文失败: %v", err)
	}
	page, err := browser.NewPage(ctx, browserContextID, "about:blank")
	if err != nil {
		t.Fatalf("打开页面失败: %v", err)
	}
	defer page.Close(ctx)

	// 注入脚本：页面加载前执行一次，确认 addScriptToEvaluateOnNewDocument 生效。
	const marker = "window.__web2apiFlowProbe = 'ok';"
	if err := page.AddScriptOnNewDocument(ctx, marker); err != nil {
		t.Fatalf("注入脚本失败: %v", err)
	}
	if err := page.Navigate(ctx, "data:text/html,<html><body>flow</body></html>", 30*time.Second); err != nil {
		t.Fatalf("导航失败: %v", err)
	}
	value, err := page.Evaluate(ctx, "window.__web2apiFlowProbe || 'missing'", false)
	if err != nil {
		t.Fatalf("执行 JS 失败: %v", err)
	}
	if !strings.Contains(value, "ok") {
		t.Fatalf("注入脚本未生效，得到 %q", value)
	}

	// Promise 求值：验证 awaitPromise 路径。
	out, err := page.Evaluate(ctx, "Promise.resolve('async-ok')", true)
	if err != nil {
		t.Fatalf("执行异步 JS 失败: %v", err)
	}
	if !strings.Contains(out, "async-ok") {
		t.Fatalf("异步求值结果异常: %q", out)
	}
}
