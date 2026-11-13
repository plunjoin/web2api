package provider

import (
	"path/filepath"
	"testing"

	"web2api/internal/store"
)

// 删除供应商的最后一个账号时，它的模型快照应一并清掉；
// 同地址还有别的 Key 时则保留。
func TestDropEmptyCatalog(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	creds := `{"base_url":"https://api.deepseek.com/v1","api_key":"k"}`
	first, err := st.CreateAccount("custom", "key-1", creds, true)
	if err != nil {
		t.Fatal(err)
	}
	second, err := st.CreateAccount("custom", "key-2", creds, true)
	if err != nil {
		t.Fatal(err)
	}
	key := "custom|https://api.deepseek.com"
	if _, err := st.ReplaceEngineCatalog(key, []store.CatalogModel{
		{Engine: key, Model: "deepseek-v4-pro"},
	}); err != nil {
		t.Fatal(err)
	}

	m := &Manager{st: st}
	removed, _ := st.GetAccount(first.ID)
	// RemoveAccount 先删库、再清理快照。
	if err := st.DeleteAccount(first.ID); err != nil {
		t.Fatal(err)
	}
	m.dropEmptyCatalog(removed)
	if n := catalogSize(t, st); n != 1 {
		t.Fatalf("同地址还有账号时快照应保留，got %d", n)
	}

	if err := st.DeleteAccount(second.ID); err != nil {
		t.Fatal(err)
	}
	m.dropEmptyCatalog(removed)
	if n := catalogSize(t, st); n != 0 {
		t.Fatalf("最后一个账号删除后快照应清空，got %d", n)
	}
}

func catalogSize(t *testing.T, st *store.Store) int {
	t.Helper()
	rows, err := st.CatalogModels()
	if err != nil {
		t.Fatal(err)
	}
	return len(rows)
}
