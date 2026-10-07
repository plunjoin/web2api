//go:build web2api_unit

package session

import (
	"context"
	"strings"
	"testing"
	"web2api/test/support/testredis"
)

func TestRedisConnectionAndCommands(t *testing.T) {
	s := testredis.Start(t)
	s.Password = "a:@complex password"
	s.Database = "2"
	r, err := New(strings.Replace(strings.TrimSuffix(s.URL(), "/0"), "redis://", "redis://user:a%3A%40complex%20password@", 1) + "/2")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if got, err := r.Command(ctx, "SET", "session", "管理员\r\nvalue", "EX", "60"); err != nil || got != "OK" {
		t.Fatalf("SET: %q %v", got, err)
	}
	if got, err := r.Command(ctx, "GET", "session"); err != nil || got != "管理员\r\nvalue" {
		t.Fatalf("GET: %q %v", got, err)
	}
	if got, err := r.Command(ctx, "DEL", "session"); err != nil || got != "1" {
		t.Fatalf("DEL: %q %v", got, err)
	}
	if got, err := r.Command(ctx, "GET", "session"); err != nil || got != "" {
		t.Fatalf("missing: %q %v", got, err)
	}
	r.password = "wrong"
	if _, err := r.Command(ctx, "GET", "session"); err == nil {
		t.Fatal("wrong redis password accepted")
	}
}

func TestInvalidRedisURL(t *testing.T) {
	for _, u := range []string{"", "http://localhost:6379", "redis:///0", "redis://localhost/-1", "redis://localhost/nope", "redis://localhost/0?unknown=1"} {
		if _, err := New(u); err == nil {
			t.Fatalf("accepted %s", u)
		}
	}
	for _, u := range []string{"redis://localhost", "rediss://user:password@localhost:6379/1", "redis://[::1]:6379/0"} {
		if _, err := New(u); err != nil {
			t.Fatalf("rejected %s: %v", u, err)
		}
	}
}
