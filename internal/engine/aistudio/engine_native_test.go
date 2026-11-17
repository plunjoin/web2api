package aistudio

import (
	"errors"
	"testing"
)

// 启动装配不完整时，管理台仍要能对残留账号操作：EnsureManaged 按邮箱补登记映射。
func TestEnsureManagedFillsMissingMapping(t *testing.T) {
	e := &NativeEngine{
		idMap: map[string]string{},
		pool:  NewAccountPool([]*Account{{ID: "a@b.c"}}, 1),
	}
	if !e.EnsureManaged("14", "a@b.c") {
		t.Fatal("池中存在同邮箱账号时应补登记映射")
	}
	if got := e.poolAccountID("14"); got != "a@b.c" {
		t.Fatalf("映射应为 a@b.c，got %q", got)
	}
	// 邮箱比较不区分大小写
	if !e.EnsureManaged("15", "A@B.C") {
		t.Fatal("大小写不同的同一邮箱也应匹配")
	}
	// 池里没有的显示名不能凭空造映射
	if e.EnsureManaged("16", "nobody@example.com") {
		t.Fatal("池中无对应账号时不应登记映射")
	}
	if got := e.poolAccountID("16"); got != "" {
		t.Fatalf("不应有映射，got %q", got)
	}
}

// 未映射的错误文案保持不变，同时可用 errors.Is 判定，供上层降级为只改存储。
func TestUnmanagedAccountError(t *testing.T) {
	e := &NativeEngine{idMap: map[string]string{}}
	err := e.SetEnabled("99", true)
	if err == nil || err.Error() != "账号 99 不在管理范围" {
		t.Fatalf("unexpected error: %v", err)
	}
	if !errors.Is(err, ErrAccountNotManaged) {
		t.Fatalf("应可用 errors.Is 判定，got %v", err)
	}
	err = e.RemoveAccount("99")
	if err == nil || err.Error() != "账号 99 不在管理范围" {
		t.Fatalf("unexpected remove error: %v", err)
	}
	if !errors.Is(err, ErrAccountNotManaged) {
		t.Fatalf("删除同样应可用 errors.Is 判定，got %v", err)
	}
}
