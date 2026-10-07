//go:build web2api_unit

package upgrade

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
		if r.URL.Query().Get("fromImage") != "ghcr.io/plunjoin/web2api" || r.URL.Query().Get("tag") != "latest" {
			authError = "image query = " + r.URL.RawQuery
		} else if got := r.Header.Get("X-Registry-Auth"); got == "" {
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

func TestParseImageRef(t *testing.T) {
	ref, err := parseImageRef("ghcr.io/plunjoin/web2api:latest")
	if err != nil || ref.registry != "ghcr.io" || ref.repository != "plunjoin/web2api" || ref.name != "ghcr.io/plunjoin/web2api" || ref.tag != "latest" {
		t.Fatalf("ghcr ref = %+v, %v", ref, err)
	}
	ref, err = parseImageRef("localhost:5000/web2api:dev")
	if err != nil || ref.registry != "localhost:5000" || ref.repository != "web2api" || ref.tag != "dev" {
		t.Fatalf("localhost ref = %+v, %v", ref, err)
	}
	ref, err = parseImageRef("ubuntu:latest")
	if err != nil || ref.registry != "docker.io" || ref.repository != "library/ubuntu" || ref.name != "docker.io/library/ubuntu" {
		t.Fatalf("docker hub ref = %+v, %v", ref, err)
	}
	query := imageCreateQuery("ghcr.io/plunjoin/web2api@sha256:abc")
	if !strings.Contains(query, "tag=sha256%3Aabc") || strings.Contains(query, "%40") {
		t.Fatalf("digest query = %s", query)
	}
}

func TestRejectedRegistryPasswordUsesAnonymousBearer(t *testing.T) {
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/":
			w.Header().Set("WWW-Authenticate", `Bearer realm="https://ghcr.io/token",service="ghcr.io"`)
			w.WriteHeader(http.StatusUnauthorized)
		case "/token":
			if r.Header.Get("Authorization") != "" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			if r.URL.Query().Get("scope") != "repository:plunjoin/web2api:pull" || r.URL.Query().Get("service") != "ghcr.io" {
				t.Errorf("token query = %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "anonymous-token"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer registry.Close()

	manager := &Manager{
		image:            "ghcr.io/plunjoin/web2api:latest",
		registryUser:     "plunjoin",
		registryPassword: "expired-token",
		registryHTTP:     &http.Client{Transport: rewriteHostTransport{base: registry.URL}},
	}
	auth, ok := manager.bearerAuth(context.Background(), manager.registryUser, manager.registryPassword)
	if !ok {
		t.Fatal("expected anonymous bearer after the registry rejected the password")
	}
	decoded, err := base64.URLEncoding.DecodeString(auth)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]string
	if err := json.Unmarshal(decoded, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["registrytoken"] != "anonymous-token" || fields["serveraddress"] != "ghcr.io" || fields["password"] != "" {
		t.Fatalf("auth = %#v", fields)
	}
}

func TestPullLatestRetriesAnonymousAfterForbidden(t *testing.T) {
	var pulls []string
	dockerAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pulls = append(pulls, r.Header.Get("X-Registry-Auth"))
		if r.Header.Get("X-Registry-Auth") != "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"message":"denied: requested access to the resource is denied"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"Download complete"}`)
	}))
	defer dockerAPI.Close()
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer registry.Close()

	encoded, err := encodeRegistryAuth(map[string]string{
		"username": "plunjoin", "password": "expired-token", "serveraddress": "ghcr.io",
	})
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{
		docker:           &dockerClient{http: &http.Client{Transport: rewriteTransport{base: dockerAPI.URL}}},
		registryHTTP:     &http.Client{Transport: rewriteHostTransport{base: registry.URL}},
		image:            "ghcr.io/plunjoin/web2api:latest",
		registryUser:     "plunjoin",
		registryPassword: "expired-token",
		registryAuth:     encoded,
	}
	if err := manager.pullLatest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(pulls) != 2 || pulls[0] == "" || pulls[1] != "" {
		t.Fatalf("pulls = %#v", pulls)
	}
}

type rewriteHostTransport struct{ base string }

func (r rewriteHostTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	target, err := url.Parse(r.base)
	if err != nil {
		return nil, err
	}
	clone.URL.Scheme = target.Scheme
	clone.URL.Host = target.Host
	return http.DefaultTransport.RoundTrip(clone)
}
