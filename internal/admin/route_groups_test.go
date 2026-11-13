package admin

import (
	"path/filepath"
	"testing"

	"web2api/internal/config"
	"web2api/internal/provider"
	"web2api/internal/store"
)

// 同名模型跨两个第三方供应商时自动成一个路由组，默认 smart，模式可保存。
func TestRouteGroupsAutoGroupSameModel(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "route-groups.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	cfg := &config.Config{Engines: config.EnginesConfig{Custom: config.CustomConfig{Enabled: true}}}
	mgr, err := provider.NewManager(cfg, st)
	if err != nil {
		t.Fatal(err)
	}
	a := &API{mgr: mgr, st: st}

	for i, base := range []string{"https://api.deepseek.com/v1", "https://relay.example.com/v1"} {
		if _, err := st.CreateAccount("custom", "key-"+string(rune('a'+i)), `{"base_url":"`+base+`","api_key":"k"}`, true); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.ReplaceEngineCatalog("custom|https://api.deepseek.com", []store.CatalogModel{
		{Engine: "custom|https://api.deepseek.com", Model: "deepseek-v4-pro", DisplayName: "DeepSeek V4 Pro"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ReplaceEngineCatalog("custom|https://relay.example.com", []store.CatalogModel{
		{Engine: "custom|https://relay.example.com", Model: "deepseek-v4-pro", DisplayName: "DeepSeek V4 Pro"},
	}); err != nil {
		t.Fatal(err)
	}

	groups := a.routeGroups()
	if len(groups) != 1 {
		t.Fatalf("应自动生成 1 个同名模型路由组，got %+v", groups)
	}
	g := groups[0]
	if g.Key != "deepseek-v4-pro" || len(g.Members) != 2 {
		t.Fatalf("组或成员不对: %+v", g)
	}
	if g.Mode != provider.RouteGroupSmart {
		t.Fatalf("默认模式应为 smart，got %q", g.Mode)
	}

	if err := mgr.SetRoutingGroupMode(g.Key, provider.RouteGroupRotate); err != nil {
		t.Fatal(err)
	}
	groups = a.routeGroups()
	if groups[0].Mode != provider.RouteGroupRotate {
		t.Fatalf("模式应保存为 rotate，got %q", groups[0].Mode)
	}
}
