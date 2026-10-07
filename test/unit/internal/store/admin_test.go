//go:build web2api_unit

package store

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
)

func TestInitializeAdminAtomic(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "init.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	v := &AdminSettings{Email: "admin@example.com", Nickname: "Admin", PasswordHash: "hash", RedisURL: "redis://localhost:6379", JWTSecret: "secret"}
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- s.InitializeAdmin(v) }()
	}
	wg.Wait()
	close(results)
	successful := 0
	for err := range results {
		if err == nil {
			successful++
		} else if !errors.Is(err, ErrInitialized) {
			t.Fatal(err)
		}
	}
	if successful != 1 {
		t.Fatalf("expected exactly one initialization: %d", successful)
	}
	other := *v
	other.Nickname = "Replacement"
	if err := s.InitializeAdmin(&other); !errors.Is(err, ErrInitialized) {
		t.Fatal(err)
	}
	saved, err := s.AdminSettings()
	if err != nil || saved.Nickname != v.Nickname {
		t.Fatalf("settings overwritten: %v %v", saved, err)
	}
}
