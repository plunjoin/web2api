package workbuddy

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStartAuthValidation(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
	}{
		{"msg", `{"code":123,"msg":"expired"}`, "expired"},
		{"message", `{"code":123,"message":"denied"}`, "denied"},
		{"missing state", `{"code":0,"data":{"authUrl":"https://example.test/login"}}`, "state"},
		{"empty URL", `{"code":0,"data":{"state":"s","authUrl":" "}}`, "authUrl"},
		{"relative URL", `{"code":0,"data":{"state":"s","authUrl":"/login"}}`, "authUrl"},
		{"unsafe URL", `{"code":0,"data":{"state":"s","authUrl":"javascript:alert(1)"}}`, "authUrl"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(tc.body)) }))
			defer srv.Close()
			_, err := StartAuth(context.Background(), "", srv.URL)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}

func TestStartAuthVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("platform") != "workbuddy" || r.URL.Query().Get("nonce") == "" {
			t.Error("missing platform or nonce")
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"state":"s","authUrl":"https://example.test/login?state=s"}}`))
	}))
	defer srv.Close()
	out, err := StartAuth(context.Background(), "", srv.URL)
	if err != nil || out.State != "s" || !strings.Contains(out.AuthURL, "version="+siteApp) {
		t.Fatalf("got %+v, %v", out, err)
	}
}

func TestPollTokenErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		status           int
		terminal         bool
	}{
		{"msg", `{"code":123,"msg":"expired"}`, "expired", 200, true},
		{"message", `{"code":123,"message":"denied"}`, "denied", 200, true},
		{"pending", `{"code":11217,"msg":"login ing..."}`, "等待用户", 200, false},
		{"server error", `unavailable`, "503", 503, false},
		{"rate limit", `limited`, "429", 429, false},
		{"rejected", `denied`, "401", 401, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			_, err := PollToken(context.Background(), "", srv.URL, "s")
			var authErr *AuthError
			if err == nil || !strings.Contains(err.Error(), tc.want) || errors.As(err, &authErr) != tc.terminal {
				t.Fatalf("got %v, want %q terminal=%v", err, tc.want, tc.terminal)
			}
			if tc.name == "pending" && !errors.Is(err, ErrPendingLogin) {
				t.Fatalf("not pending: %v", err)
			}
		})
	}
}
