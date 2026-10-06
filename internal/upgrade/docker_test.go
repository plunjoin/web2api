package upgrade

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegistryAddress(t *testing.T) {
	for _, test := range []struct {
		image, want string
	}{
		{"ghcr.io/plunjoin/web2api:latest", "ghcr.io"},
		{"registry.example/web2api:latest", "registry.example"},
		{"localhost:5000/web2api:latest", "localhost:5000"},
		{"plunjoin/web2api:latest", "docker.io"},
	} {
		if got := registryAddress(test.image); got != test.want {
			t.Errorf("registryAddress(%q) = %q, want %q", test.image, got, test.want)
		}
	}
}

func TestPullSendsRegistryAddressAndReportsDockerError(t *testing.T) {
	authErrors := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authError := ""
		if got := r.Header.Get("X-Registry-Auth"); got == "" {
			authError = "pull did not send registry credentials"
		} else {
			decoded, err := base64.URLEncoding.DecodeString(got)
			if err != nil {
				authError = "decode registry credentials: " + err.Error()
			}
			var auth struct {
				ServerAddress string `json:"serveraddress"`
			}
			if err := json.Unmarshal(decoded, &auth); err != nil {
				authError = "decode registry credentials JSON: " + err.Error()
			}
			if auth.ServerAddress != "ghcr.io" {
				authError = "registry address = " + auth.ServerAddress + ", want ghcr.io"
			}
		}
		authErrors <- authError
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"message":"denied: requested access to the resource is denied"}`)
	}))
	defer server.Close()

	client := &dockerClient{http: &http.Client{Transport: rewriteTransport{base: server.URL}}}
	err := client.pull(context.Background(), "ghcr.io/plunjoin/web2api:latest", base64.URLEncoding.EncodeToString([]byte(`{"serveraddress":"ghcr.io"}`)))
	if authError := <-authErrors; authError != "" {
		t.Fatal(authError)
	}
	if err == nil || !strings.Contains(err.Error(), "requested access to the resource is denied") {
		t.Fatalf("pull error = %v", err)
	}
}
