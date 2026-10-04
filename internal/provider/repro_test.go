package provider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"web2api/internal/config"
)

// TestReproduceDisableCycle 复现「停用→启用→停用→删除」场景下的租约占用。
func TestReproduceDisableCycle(t *testing.T) {
	dir := t.TempDir()
	authRoot := filepath.Join(dir, "auth")
	cfg := config.EngineBConfig{
		Enabled:               true,
		Mode:                  "native",
		AuthStates:            authRoot,
		PerAccountConcurrency: 2,
		InitTimeoutSeconds:    5,
		RequestTimeoutSeconds: 10,
		RefreshSeconds:        300,
	}
	engineB, err := NewNativeAIStudio(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := engineB.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer engineB.Close()

	storage := `{"cookies":[{"name":"SID","value":"x","domain":".google.com","path":"/","expires":4102444800,"httpOnly":true,"secure":true}],"origins":[]}`
	if err := engineB.AddAccount("1", "cycle@gmail.com", storage, "", "", ""); err != nil {
		t.Fatalf("添加: %v", err)
	}
	if !engineB.Ready() {
		t.Fatal("热添加账号后引擎应立即就绪")
	}
	dump := func(stage string) {
		for _, st := range engineB.AccountStates() {
			fmt.Printf("[%s] id=%s label=%s status=%s detail=%q models=%d\n", stage, st.ID, st.Label, st.Status, st.Detail, st.Models)
		}
	}
	dump("添加后")

	if err := engineB.SetEnabled("1", false); err != nil {
		t.Fatalf("停用: %v", err)
	}
	dump("停用后")
	if err := engineB.SetEnabled("1", true); err != nil {
		t.Fatalf("启用: %v", err)
	}
	dump("启用后")
	if err := engineB.SetEnabled("1", false); err != nil {
		t.Fatalf("二次停用: %v", err)
	}
	dump("二次停用后")
	if err := engineB.RemoveAccount("1"); err != nil {
		t.Fatalf("删除: %v", err)
	}
	dump("删除后")

	if _, err := os.Stat(filepath.Join(authRoot, "cycle@gmail.com")); !os.IsNotExist(err) {
		t.Fatalf("凭据目录应已删除")
	}
}
