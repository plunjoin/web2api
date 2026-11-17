package provider

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"web2api/internal/engine"
	"web2api/internal/engine/aistudio"
	"web2api/internal/store"
)

// staleAIStudioPool 模拟启动装配失败/凭据损坏后库里残留的 AI Studio 账号：
// 引擎的 ID 映射里没有它，删除与启停都会报 ErrAccountNotManaged。
type staleAIStudioPool struct {
	removeCalls int
	enableCalls int
	managed     map[string]string
}

func (p *staleAIStudioPool) RemoveAccount(engineID string) error {
	p.removeCalls++
	if _, ok := p.managed[engineID]; ok {
		return nil
	}
	return fmt.Errorf("账号 %s %w", engineID, aistudio.ErrAccountNotManaged)
}

func (p *staleAIStudioPool) SetEnabled(engineID string, _ bool) error {
	p.enableCalls++
	if _, ok := p.managed[engineID]; ok {
		return nil
	}
	return fmt.Errorf("账号 %s %w", engineID, aistudio.ErrAccountNotManaged)
}

func (p *staleAIStudioPool) AccountStates() []engine.AccountState { return nil }

// EnsureManaged 模拟按显示名补登记映射：池里其实没有邮箱账号，所以永远补不上。
func (p *staleAIStudioPool) EnsureManaged(managedID, poolID string) bool {
	if _, ok := p.managed[managedID]; ok {
		return true
	}
	return false
}

// AI Studio 原生引擎用「不在管理范围」表示管理 ID 未映射，和其他订阅引擎的
// 「不在池中」不同；两者都必须被当作「账号已不在引擎掌握中」，
// 否则装配失败的残留账号在管理台永远删不掉（报「账号 N 不在管理范围」）。
func TestPoolAccountMissing(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"aistudio 未映射", fmt.Errorf("账号 14 %w", aistudio.ErrAccountNotManaged), true},
		{"订阅引擎不在池中", errors.New("账号 7 不在池中"), true},
		{"账号被占用", errors.New("账号正被后台刷新或活跃请求占用"), false},
		{"目录清理失败", errors.New("清理账号目录失败: permission denied"), false},
		{"无错误", nil, false},
	}
	for _, tc := range cases {
		if got := poolAccountMissing(tc.err); got != tc.want {
			t.Errorf("%s: poolAccountMissing(%v) = %v, want %v", tc.name, tc.err, got, tc.want)
		}
	}
}

// 未映射的 AI Studio 账号仍可删除：引擎报错被降级，库里的记录照常删除。
func TestRemoveUnmanagedAIStudioAccountDeletesRecord(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "accounts.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	acc, err := st.CreateAccount("aistudio", "stale@example.com", `{"storage_state":{}}`, true)
	if err != nil {
		t.Fatal(err)
	}
	pool := &staleAIStudioPool{managed: map[string]string{}}
	m := &Manager{st: st, pools: map[string]accountPool{"aistudio": pool}}

	if err := m.RemoveAccount(acc.ID); err != nil {
		t.Fatalf("残留账号应可删除，got %v", err)
	}
	if pool.removeCalls != 1 {
		t.Fatalf("应尝试出池一次，got %d", pool.removeCalls)
	}
	if _, err := st.GetAccount(acc.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("库记录应已删除，got %v", err)
	}
}

// 反向导入必须用刚创建的 aistudio 行 ID 登记映射。旧实现取 ListAccounts
// 末尾的 ID（排序键 engine,id），多引擎混排时会拿到别的引擎的账号，
// 导致导入的账号全部没有映射、在管理台报「不在管理范围」。
func TestImportPoolAccountsRegistersCreatedRowID(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "accounts.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	other, err := st.CreateAccount("workbuddy", "buddy", `{"token":"t"}`, true)
	if err != nil {
		t.Fatal(err)
	}
	importer := &fakePoolImporter{
		ids:        []string{"first@example.com", "second@example.com"},
		registered: map[string]string{},
	}
	m := &Manager{st: st}
	m.importPoolAccounts(importer)

	accounts, err := st.ListAccounts()
	if err != nil {
		t.Fatal(err)
	}
	var imported []store.Account
	for _, acc := range accounts {
		if acc.Engine == "aistudio" {
			imported = append(imported, acc)
		}
	}
	if len(imported) != 2 {
		t.Fatalf("应补建两条 AI Studio 记录，got %d", len(imported))
	}
	for _, acc := range imported {
		if got := importer.registered[fmt.Sprintf("%d", acc.ID)]; got != acc.Label {
			t.Errorf("账号 %d 的映射应为 %q，got %q（全部映射: %v）", acc.ID, acc.Label, got, importer.registered)
		}
	}
	if _, ok := importer.registered[fmt.Sprintf("%d", other.ID)]; ok {
		t.Fatal("不应把其他引擎的账号 ID 登记成 AI Studio 映射")
	}
}

type fakePoolImporter struct {
	ids        []string
	registered map[string]string
}

func (f *fakePoolImporter) PoolAccountIDs() []string { return f.ids }

func (f *fakePoolImporter) RegisterAccountID(managedID, aistudioID string) {
	f.registered[managedID] = aistudioID
}

// 未映射的 AI Studio 账号仍可启停：引擎报错被降级，存储记录照常更新。
func TestSetEnabledUnmanagedAIStudioAccountUpdatesStore(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "accounts.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	acc, err := st.CreateAccount("aistudio", "stale@example.com", `{"storage_state":{}}`, true)
	if err != nil {
		t.Fatal(err)
	}
	pool := &staleAIStudioPool{managed: map[string]string{}}
	m := &Manager{st: st, pools: map[string]accountPool{"aistudio": pool}}

	if err := m.SetAccountEnabled(acc.ID, false); err != nil {
		t.Fatalf("残留账号应可停用，got %v", err)
	}
	if pool.enableCalls != 1 {
		t.Fatalf("应尝试下发停用一次，got %d", pool.enableCalls)
	}
	got, err := st.GetAccount(acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Enabled {
		t.Fatal("存储记录应已停用")
	}
}
