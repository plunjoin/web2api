package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"web2api/internal/engine/workbuddy"
	"web2api/internal/provider"
	"web2api/internal/store"
)

func TestWorkBuddyPollMissingAndExpired(t *testing.T) {
	a := &API{}
	// A nil manager ensures missing sessions cannot fall back to configured endpoints.
	r := httptest.NewRequest(http.MethodPost, "/admin/api/oauth/workbuddy/poll", strings.NewReader(`{"session_id":"missing"}`))
	r.SetPathValue("engine", "workbuddy")
	w := httptest.NewRecorder()
	a.handleOAuthPoll(w, r)
	if w.Code != http.StatusGone {
		t.Fatalf("missing session: %d %s", w.Code, w.Body.String())
	}
	a.saveWorkBuddySession("expired", "http://127.0.0.1:1", "bad-proxy")
	a.workBuddySessions["expired"].createdAt = time.Now().Add(-31 * time.Minute)
	_, status := a.pollWorkBuddy(context.Background(), "expired", "", "", func(provider.AccountInput) (store.Account, error) {
		t.Fatal("expired session must not add account")
		return store.Account{}, nil
	})
	if status != http.StatusGone {
		t.Fatalf("expired session: %d", status)
	}
}

func TestWorkBuddyPollConcurrentCompletion(t *testing.T) {
	var polls, adds atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/plugin/auth/token":
			polls.Add(1)
			if r.URL.Query().Get("state") != "session" {
				t.Error("wrong state")
			}
			select {
			case <-started:
			default:
				close(started)
			}
			<-release
			_, _ = w.Write([]byte(`{"code":0,"data":{"accessToken":"e30.e30.signature","refreshToken":"rt"}}`))
		case "/v2/plugin/login/account":
			_, _ = w.Write([]byte(`{"data":{"uid":"user"}}`))
		default:
			t.Errorf("unexpected upstream path: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	a := &API{}
	a.saveWorkBuddySession("session", srv.URL, "")
	add := func(input provider.AccountInput) (store.Account, error) {
		adds.Add(1)
		if input.Label != "custom label" {
			t.Errorf("label lost: %q", input.Label)
		}
		var c workbuddy.Credentials
		if err := json.Unmarshal(input.Credentials, &c); err != nil {
			t.Error(err)
		}
		if c.BaseURL != srv.URL || c.Proxy != "" || c.UID != "user" {
			t.Errorf("session identity lost: %+v", c)
		}
		return store.Account{ID: 42, Label: input.Label}, nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, status := a.pollWorkBuddy(context.Background(), "session", "custom label", "", add)
			if status != http.StatusOK || result["account"].(store.Account).ID != 42 {
				t.Errorf("bad completion: %d %+v", status, result)
			}
		}()
	}
	<-started
	close(release)
	wg.Wait()
	if polls.Load() != 1 || adds.Load() != 1 {
		t.Fatalf("polls=%d adds=%d", polls.Load(), adds.Load())
	}
}

func TestWorkBuddyPollRetryAndDefaultLabel(t *testing.T) {
	var polls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/plugin/login/account" {
			_, _ = w.Write([]byte(`{"data":{}}`))
			return
		}
		switch polls.Add(1) {
		case 1:
			w.WriteHeader(http.StatusServiceUnavailable)
		case 2:
			_, _ = w.Write([]byte(`{"code":11217,"msg":"login ing..."}`))
		default:
			_, _ = w.Write([]byte(`{"code":0,"data":{"accessToken":"e30.e30.signature"}}`))
		}
	}))
	defer srv.Close()
	a := &API{}
	a.saveWorkBuddySession("s", srv.URL, "")
	add := func(input provider.AccountInput) (store.Account, error) {
		if input.Label == "" {
			t.Error("missing automatic label")
		}
		return store.Account{ID: 1}, nil
	}
	for _, want := range []int{http.StatusBadGateway, http.StatusAccepted, http.StatusOK, http.StatusOK} {
		_, status := a.pollWorkBuddy(context.Background(), "s", "", "", add)
		if status != want {
			t.Fatalf("got %d want %d", status, want)
		}
	}
	if polls.Load() != 3 {
		t.Fatalf("completed session polled again: %d", polls.Load())
	}
}
