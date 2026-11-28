package store

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSetAccountProjectIDLeavesCookies(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "project.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	original := `{"cookies":"OSID=live; SID=account","email":"a@example.com"}`
	account, err := st.CreateAccount("flow", "main", original, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetAccountProjectID(account.ID, "project-9"); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetAccount(account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "initializing" {
		t.Fatalf("status=%q, want initializing", got.Status)
	}
	if !strings.Contains(got.Credentials, "OSID=live") || !strings.Contains(got.Credentials, "project-9") {
		t.Fatalf("credentials=%s", got.Credentials)
	}
}
