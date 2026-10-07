package test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"web2api/internal/store"
)

func TestLegacyCookieImportPreservesCacheFiles(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "migration.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	account, err := st.CreateAccount("a", "web", `{"psid":"migration-psid","psidts":"base"}`, true)
	if err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(dir, "legacy")
	if err := os.MkdirAll(cache, 0o700); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(cache, ".cached_cookies_migration-psid.json")
	data := []byte(`[{"name":"__Secure-1PSID","value":"migration-psid","domain":".google.com","path":"/"},{"name":"__Secure-1PSIDTS","value":"rotated","domain":".google.com","path":"/"}]`)
	if err := os.WriteFile(filename, data, 0o600); err != nil {
		t.Fatal(err)
	}
	count, err := st.ImportLegacyCookies(cache)
	if err != nil || count != 1 {
		t.Fatalf("import: %d %v", count, err)
	}
	got, err := st.LoadCookieSession(account.ID, "migration-psid", "base")
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("import did not preserve cookie data", err)
	}
	if _, err := os.Stat(filename); err != nil {
		t.Fatal("migration removed original file", err)
	}
}

func TestCookieSessionsSurviveRestartAndCredentialReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cookies.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	account, err := st.CreateAccount("a", "web", `{"psid":"psid","psidts":"original"}`, true)
	if err != nil {
		t.Fatal(err)
	}
	jar := []byte(`[{"name":"__Secure-1PSID","value":"psid","domain":".google.com","path":"/"},{"name":"__Secure-1PSIDTS","value":"rotated","domain":".google.com","path":"/"}]`)
	if err := st.SaveCookieSession(account.ID, "psid", "original", jar); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
	st, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	got, err := st.LoadCookieSession(account.ID, "psid", "original")
	if err != nil || !bytes.Equal(got, jar) {
		t.Fatalf("rotated cookie lost on restart: %s %v", got, err)
	}
	if err := st.UpdateAccountCredentials(account.ID, `{"psid":"psid","psidts":"replacement"}`); err != nil {
		t.Fatal(err)
	}
	got, err = st.LoadCookieSession(account.ID, "psid", "replacement")
	if err != nil || len(got) != 0 {
		t.Fatal("old session survived credential replacement")
	}
	// A refresh from the replaced client must not restore stale credentials.
	if err := st.SaveCookieSession(account.ID, "psid", "original", jar); err != nil {
		t.Fatal(err)
	}
	got, _ = st.LoadCookieSession(account.ID, "psid", "replacement")
	if len(got) != 0 {
		t.Fatal("stale refresh revived session")
	}
	if err := st.SaveCookieSession(account.ID, "psid", "replacement", jar); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteAccount(account.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = st.LoadCookieSession(account.ID, "psid", "replacement")
	if len(got) != 0 {
		t.Fatal("deleted account session retained")
	}
	if err := st.SaveCookieSession(account.ID, "psid", "replacement", jar); err != nil {
		t.Fatal(err)
	}
	got, _ = st.LoadCookieSession(account.ID, "psid", "replacement")
	if len(got) != 0 {
		t.Fatal("refresh revived deleted account")
	}
}

func TestExpiredUploadSessionsAreUnavailable(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "uploads.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.SaveGeminiUploadURL("old", "https://example.com/upload", time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GeminiUploadURL("old"); err == nil {
		t.Fatal("expired upload session returned")
	}
}
