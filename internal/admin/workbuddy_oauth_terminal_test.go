package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"web2api/internal/provider"
	"web2api/internal/store"
)

func TestWorkBuddyPollTerminalRejection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":123,"msg":"expired"}`))
	}))
	defer srv.Close()
	a := &API{}
	a.saveWorkBuddySession("s", srv.URL, "")
	result, status := a.pollWorkBuddy(context.Background(), "s", "", "", func(provider.AccountInput) (store.Account, error) {
		t.Fatal("terminal rejection must not add account")
		return store.Account{}, nil
	})
	if status != http.StatusBadRequest || result["error"] != "授权未完成（code=123）: expired" {
		t.Fatalf("got %d %+v", status, result)
	}
}
