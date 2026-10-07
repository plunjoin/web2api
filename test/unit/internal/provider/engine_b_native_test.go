//go:build web2api_unit

package provider

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"web2api/internal/config"
	"web2api/internal/store"
)

// TestNativeAIStudioEmptyAuth 验证 native 引擎在空 auth 目录下完成装配（空池可用，管理台可加号）。
func TestNativeAIStudioEmptyAuth(t *testing.T) {
	dir := t.TempDir()
	cfg := config.EngineBConfig{
		Enabled:    true,
		Mode:       "native",
		AuthStates: filepath.Join(dir, "auth"),
	}
	engineB, err := NewNativeAIStudio(cfg)
	if err != nil {
		t.Fatalf("构建原生引擎失败: %v", err)
	}
	if engineB == nil {
		t.Fatal("引擎不应为 nil")
	}
	if err := engineB.Init(context.Background()); err != nil {
		t.Fatalf("空 auth 目录应完成装配而非报错: %v", err)
	}
	if engineB.Ready() {
		t.Fatal("空目录时引擎不应就绪")
	}
	// 空池状态下 AddAccount 应能走到凭据校验（而非"账户未初始化"崩溃路径）
	if err := engineB.AddAccount("smoke-id", "someone@gmail.com", `{"cookies":[],"origins":[]}`, "", "", ""); err == nil {
		t.Log("空 storage-state 通过格式校验（预期）")
	} else {
		t.Logf("凭据校验结果: %v", err)
	}
	engineB.Close()
}

// TestNativeAIStudioWithFakeAccount 验证带假账号文件的装配链（不发起生成请求）。
func TestNativeAIStudioWithFakeAccount(t *testing.T) {
	dir := t.TempDir()
	authDir := filepath.Join(dir, "auth", "test@example.com")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// AIStudio2API 兼容的账号结构：storage-state.json + account.json(config)
	storage := `{"cookies":[{"name":"SID","value":"test","domain":".google.com","path":"/","expires":4102444800,"httpOnly":true,"secure":true}],"origins":[]}`
	if err := os.WriteFile(filepath.Join(authDir, "storage-state.json"), []byte(storage), 0o600); err != nil {
		t.Fatal(err)
	}
	// AccountConfig 结构参考 AIStudio2API 的 auth/<email>/account.json
	// Label 必须是 Google 邮箱；Locale/Timezone 必填（Validate 强制）
	accountCfg := `{"enabled":true,"label":"test@gmail.com","proxy":"","locale":"en-US","timezone":"America/New_York"}`
	if err := os.WriteFile(filepath.Join(authDir, "account.json"), []byte(accountCfg), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.EngineBConfig{
		Enabled:               true,
		Mode:                  "native",
		AuthStates:            filepath.Join(dir, "auth"),
		PerAccountConcurrency: 1,
		InitTimeoutSeconds:    5,
		RequestTimeoutSeconds: 30,
	}
	engineB, err := NewNativeAIStudio(cfg)
	if err != nil {
		t.Fatalf("构建原生引擎失败: %v", err)
	}
	// Init：装配链会构建完成，模型目录拉取因假 Cookie 失败但不阻断装配
	initErr := engineB.Init(context.Background())
	if initErr != nil {
		t.Logf("Init 返回错误（装配链中断）: %v", initErr)
	}
	defer engineB.Close()

	status := engineB.Status()
	accounts, _ := status["accounts"].([]map[string]any)
	if len(accounts) == 0 {
		t.Fatalf("应加载到 1 个账号: %v", status)
	}
	if mode := status["mode"]; mode != "native" {
		t.Fatalf("模式应为 native: %v", mode)
	}
	if engineB.Passthrough() {
		t.Fatal("native 模式不应透传")
	}
}

// TestManagerNativeMode 验证 Manager 按 mode 构建引擎。
func TestManagerNativeMode(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		Server:  config.ServerConfig{Listen: "127.0.0.1:0"},
		Routing: config.RoutingConfig{DefaultEngine: "auto"},
		EngineB: config.EngineBConfig{
			Enabled:    true,
			Mode:       "native",
			AuthStates: filepath.Join(dir, "auth"),
		},
	}
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	mgr, err := NewManager(cfg, st)
	if err != nil {
		t.Fatal(err)
	}
	if mgr.EngineB() == nil {
		t.Fatal("引擎B 应已构建")
	}
	if mgr.EngineB().Describe() == "" {
		t.Fatal("引擎描述为空")
	}
}
