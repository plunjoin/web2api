package store

import (
	"path/filepath"
	"testing"
)

func TestUpdateAccountCredentialsIfCookiesMatch(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "cookies.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	original := `{"cookies":"OSID=old; SID=account","email":"a@example.com","project_id":"p1"}`
	account, err := st.CreateAccount("flow", "main", original, true)
	if err != nil {
		t.Fatal(err)
	}
	refreshed := `{"cookies":"OSID=new; SID=account","email":"a@example.com","project_id":"p1"}`
	wrote, err := st.UpdateAccountCredentialsIfCookiesMatch(account.ID, "OSID=old; SID=account", refreshed)
	if err != nil || !wrote {
		t.Fatalf("wrote=%v err=%v, want a match", wrote, err)
	}
	got, err := st.GetAccount(account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "initializing" {
		t.Fatalf("status=%q, want initializing", got.Status)
	}
	if got.Credentials != refreshed {
		t.Fatalf("credentials=%s", got.Credentials)
	}
	stale := `{"cookies":"OSID=stale; SID=account"}`
	wrote, err = st.UpdateAccountCredentialsIfCookiesMatch(account.ID, "OSID=old; SID=account", stale)
	if err != nil {
		t.Fatal(err)
	}
	if wrote {
		t.Fatal("stale snapshot should not match")
	}
	got, err = st.GetAccount(account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Credentials != refreshed {
		t.Fatalf("credentials overwritten: %s", got.Credentials)
	}
}
