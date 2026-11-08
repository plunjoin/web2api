package store

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestAccountLabelConflictAndSafeNumbering(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "labels.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	original, err := st.CreateAccount("workbuddy", "same", `{"token":"original"}`, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAccount("workbuddy", "same", "{}", true); !errors.Is(err, ErrAccountLabelExists) || strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("want friendly typed conflict, got %v", err)
	}
	if _, err := st.CreateAccount("workbuddy", "same (2)", "{}", true); err != nil {
		t.Fatal(err)
	}
	got, err := st.CreateAccountWithUniqueLabel("workbuddy", " same ", `{"token":"new"}`, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Label != "same (3)" || got.ID == original.ID || !got.Enabled || got.Credentials != `{"token":"new"}` {
		t.Fatalf("unexpected new account: id=%d label=%q", got.ID, got.Label)
	}
	old, err := st.GetAccount(original.ID)
	if err != nil || old != original {
		t.Fatal("existing account must remain unchanged")
	}
	other, err := st.CreateAccountWithUniqueLabel("custom", "same", "{}", false)
	if err != nil || other.Label != "same" || other.Enabled {
		t.Fatalf("engine scope or enabled flag changed: %v", err)
	}
}

func TestAccountUniqueLabelConcurrentCreation(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "concurrent.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	const count = 8
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := st.CreateAccountWithUniqueLabel("workbuddy", "login", "{}", true); err != nil {
				t.Errorf("concurrent create: %v", err)
			}
		}()
	}
	wg.Wait()
	accounts, err := st.ListAccounts()
	if err != nil || len(accounts) != count {
		t.Fatalf("accounts=%d want %d: %v", len(accounts), count, err)
	}
	labels := map[string]bool{}
	for _, acc := range accounts {
		if labels[acc.Label] {
			t.Fatalf("duplicate label %q", acc.Label)
		}
		labels[acc.Label] = true
	}
	for n := 1; n <= count; n++ {
		label := "login"
		if n > 1 {
			label = fmt.Sprintf("login (%d)", n)
		}
		if !labels[label] {
			t.Errorf("missing label %q", label)
		}
	}
}

func TestAccountUniqueLabelDoesNotHideOtherErrors(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "errors.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []struct{ engine, label string }{{"", "name"}, {"workbuddy", " "}} {
		if _, err := st.CreateAccountWithUniqueLabel(input.engine, input.label, "{}", true); err == nil || errors.Is(err, ErrAccountLabelExists) {
			t.Fatalf("validation error lost: %v", err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAccountWithUniqueLabel("workbuddy", "name", "{}", true); err == nil || errors.Is(err, ErrAccountLabelExists) {
		t.Fatalf("database error lost: %v", err)
	}
}
