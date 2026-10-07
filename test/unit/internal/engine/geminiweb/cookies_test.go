//go:build web2api_unit

package geminiweb

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

type memoryCookies struct {
	data []byte
	err  error
}

func (m *memoryCookies) LoadCookies() ([]byte, error) { return m.data, m.err }
func (m *memoryCookies) SaveCookies(data []byte) error {
	m.data = append([]byte(nil), data...)
	return m.err
}
func (m *memoryCookies) DeleteCookies() error { m.data = nil; return m.err }

func TestPersistentJarFiltersExpiryDomainsAndRemainsThreadSafe(t *testing.T) {
	jar := newCookieJar()
	jar.Set("__Secure-1PSID", "psid")
	jar.Set("__Secure-1PSIDTS", "fresh")
	jar.SetFull(Cookie{Name: "expired", Value: "gone", Domain: ".google.com", Expires: float64(time.Now().Add(-time.Hour).Unix())})
	jar.SetFull(Cookie{Name: "foreign", Value: "gone", Domain: "example.com"})
	storage := &memoryCookies{}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				jar.Set("other", "v")
				_ = jar.Header()
				_ = jar.snapshot()
			}
		}()
	}
	for i := 0; i < 100; i++ {
		if err := saveCookies(storage, jar); err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()
	var list []Cookie
	if err := json.Unmarshal(storage.data, &list); err != nil {
		t.Fatal(err)
	}
	for _, cookie := range list {
		if cookie.Name == "foreign" || cookie.Name == "expired" {
			t.Fatal("invalid cookie persisted", cookie.Name)
		}
	}
	loaded, err := loadCachedJar(storage)
	if err != nil || loaded.Get("__Secure-1PSIDTS") != "fresh" {
		t.Fatalf("session not restored: %v", err)
	}
	storage.err = errors.New("sqlite unavailable")
	if _, err := loadCachedJar(storage); err == nil {
		t.Fatal("persistence error ignored")
	}
}
