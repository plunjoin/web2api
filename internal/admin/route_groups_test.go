package admin

import (
	"encoding/json"
	"net/http/httptest"
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

// 停用账号后，该账号所属供应商的模型与路由组成员都要消失；只剩一个供应商时
// 路由组本身也不再成立。重新启用后恢复。
func TestRouteGroupsDropDisabledAccountProvider(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "disabled-account.db"))
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

	bases := []string{"https://api.deepseek.com/v1", "https://relay.example.com/v1"}
	ids := make([]int64, 0, len(bases))
	for i, base := range bases {
		acc, err := st.CreateAccount("custom", "key-"+string(rune('a'+i)), `{"base_url":"`+base+`","api_key":"k"}`, true)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, acc.ID)
		key := "custom|" + []string{"https://api.deepseek.com", "https://relay.example.com"}[i]
		if _, err := st.ReplaceEngineCatalog(key, []store.CatalogModel{
			{Engine: key, Model: "deepseek-v4-pro", DisplayName: "DeepSeek V4 Pro"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if groups := a.routeGroups(); len(groups) != 1 || len(groups[0].Members) != 2 {
		t.Fatalf("停用前应有一个 2 成员路由组，got %+v", groups)
	}

	if err := st.SetAccountEnabled(ids[0], false); err != nil {
		t.Fatal(err)
	}
	if groups := a.routeGroups(); len(groups) != 0 {
		t.Fatalf("停用其中一个账号后，只剩一个供应商，路由组应消失，got %+v", groups)
	}
	if active := a.activeProviders(); len(active) != 1 {
		t.Fatalf("停用账号的供应商不应再算活跃，got %v", active)
	}
	if _, ok := a.activeProviders()["custom|https://api.deepseek.com"]; ok {
		t.Fatal("停用账号的供应商仍出现在活跃列表里")
	}

	rec := httptest.NewRecorder()
	a.handleModels(rec, httptest.NewRequest("GET", "/admin/api/models", nil))
	var modelsResp struct {
		Models []struct {
			Engine string `json:"engine"`
		} `json:"models"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &modelsResp); err != nil {
		t.Fatalf("decode models: %v", err)
	}
	if len(modelsResp.Models) != 1 || modelsResp.Models[0].Engine != "custom|https://relay.example.com" {
		t.Fatalf("停用账号的供应商模型不应出现，got %+v", modelsResp.Models)
	}

	if err := st.SetAccountEnabled(ids[0], true); err != nil {
		t.Fatal(err)
	}
	if groups := a.routeGroups(); len(groups) != 1 || len(groups[0].Members) != 2 {
		t.Fatalf("重新启用后路由组应恢复，got %+v", groups)
	}
}

// TestRouteGroupsSplitWorkBuddyEditions 国服与国际服是两个供应商：同名模型在路由组里
// 分成 workbuddy 与 workbuddy-ai 两个成员。
func TestRouteGroupsSplitWorkBuddyEditions(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "wb-editions.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	enabled := true
	cfg := &config.Config{Engines: config.EnginesConfig{WorkBuddy: config.WorkBuddyConfig{Enabled: enabled}}}
	mgr, err := provider.NewManager(cfg, st)
	if err != nil {
		t.Fatal(err)
	}
	a := &API{mgr: mgr, st: st}

	if _, err := st.CreateAccount("workbuddy", "cn", `{"token":"t","site":"workbuddy"}`, true); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAccount("workbuddy-ai", "intl", `{"token":"t","site":"workbuddy-ai"}`, true); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"workbuddy", "workbuddy-ai"} {
		if _, err := st.ReplaceEngineCatalog(key, []store.CatalogModel{
			{Engine: key, Model: "glm-5.3", DisplayName: "GLM-5.3"},
		}); err != nil {
			t.Fatal(err)
		}
	}

	groups := a.routeGroups()
	var got *routeGroupView
	for i := range groups {
		if groups[i].Key == "glm-5.3" {
			got = &groups[i]
		}
	}
	if got == nil || len(got.Members) != 2 {
		t.Fatalf("glm-5.3 应由国服与国际服两个成员组成，got %+v", groups)
	}
	keys := map[string]bool{}
	for _, m := range got.Members {
		keys[m.Key] = true
	}
	if !keys["workbuddy"] || !keys["workbuddy-ai"] {
		t.Fatalf("成员应为两个发行版，got %+v", got.Members)
	}
}
