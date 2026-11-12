package store

import (
	"path/filepath"
	"testing"
)

// TestNormalizeLegacyEnginesMergesDuplicates 旧引擎名（b）与新名（aistudio）各存一行
// 时，应合并成一行：保留规范行、带走真实凭据、启停取「或」，历史行删除。
// 否则管理台会出现两条一模一样的账号，其中一条永远「未加载」。
func TestNormalizeLegacyEnginesMergesDuplicates(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	legacy, err := st.CreateAccount("b", "a@b.c", `{"storage_state":{"cookies":[1]}}`, true)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := st.CreateAccount("aistudio", "a@b.c", "{}", false)
	if err != nil {
		t.Fatal(err)
	}

	n, err := st.NormalizeLegacyEngines()
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1 record normalized, got %d", n)
	}
	if _, err := st.GetAccount(legacy.ID); err == nil {
		t.Fatal("历史行应被删除")
	}
	got, err := st.GetAccount(canonical.ID)
	if err != nil {
		t.Fatalf("规范行应保留: %v", err)
	}
	if got.Engine != "aistudio" {
		t.Errorf("engine = %q, want aistudio", got.Engine)
	}
	if got.Credentials != `{"storage_state":{"cookies":[1]}}` {
		t.Errorf("凭据应来自有内容的一侧，got %q", got.Credentials)
	}
	if !got.Enabled {
		t.Error("启停应取「或」：任一启用即启用")
	}
}

// 只有历史行、没有冲突时直接改名，不删记录。
func TestNormalizeLegacyEnginesRenames(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	old, err := st.CreateAccount("a", "x@y.z", `{"psid":"p"}`, true)
	if err != nil {
		t.Fatal(err)
	}
	// 查询走 scanAccount，引擎名读出时就已归一，这里检查的是库里写回的值。
	raw := struct {
		engine string
	}{}
	if err := st.db.QueryRow(`SELECT engine FROM accounts WHERE id = ?`, old.ID).Scan(&raw.engine); err != nil {
		t.Fatal(err)
	}
	if raw.engine != "a" {
		t.Fatalf("precondition: stored engine = %q, want a", raw.engine)
	}
	if n, err := st.NormalizeLegacyEngines(); err != nil || n != 1 {
		t.Fatalf("normalize = %d, %v", n, err)
	}
	if err := st.db.QueryRow(`SELECT engine FROM accounts WHERE id = ?`, old.ID).Scan(&raw.engine); err != nil {
		t.Fatal(err)
	}
	if raw.engine != "geminiweb" {
		t.Errorf("stored engine = %q, want geminiweb", raw.engine)
	}
}

// 已经是规范名的记录不受影响。
func TestNormalizeLegacyEnginesNoop(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	acc, err := st.CreateAccount("workbuddy", "w@x.y", `{"token":"t"}`, true)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := st.NormalizeLegacyEngines(); err != nil || n != 0 {
		t.Fatalf("normalize = %d, %v; want 0", n, err)
	}
	got, err := st.GetAccount(acc.ID)
	if err != nil || got.Engine != "workbuddy" {
		t.Fatalf("account changed unexpectedly: %+v %v", got, err)
	}
}

// 路由顺序按原始字符串保存并原样读回；未配置时是 nil（走默认顺序）。
func TestRoutingAccountOrderRoundTrip(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	ids, err := st.RoutingAccountOrder()
	if err != nil || ids != nil {
		t.Fatalf("unset order = %v, %v; want nil", ids, err)
	}
	if err := st.SetRoutingAccountOrder([]string{"3", "1", "2"}); err != nil {
		t.Fatal(err)
	}
	ids, err = st.RoutingAccountOrder()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 3 || ids[0] != "3" || ids[1] != "1" || ids[2] != "2" {
		t.Fatalf("order = %v", ids)
	}
}
