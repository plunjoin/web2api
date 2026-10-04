package upgrade

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeDocker struct {
	mu                                            sync.Mutex
	old                                           containerInfo
	requests                                      []string
	helper, replacement                           map[string]any
	pullError, failStart                          bool
	health                                        string
	pruneUntagged, currentPinned, oldImageRemoved bool
}

func newFakeDocker(t *testing.T) (*Manager, *fakeDocker) {
	t.Helper()
	dir := filepath.ToSlash(t.TempDir())
	var old containerInfo
	fixture := `{
      "Id":"old-container-id", "Name":"/web2api", "Image":"sha256:old",
      "Config":{"Image":"old-tag", "Hostname":"old-containe", "User":"", "Env":["CUSTOM=value"], "Cmd":["-config","/data/config.yaml"], "Labels":{"com.docker.compose.service":"web2api","org.opencontainers.image.version":"v0.1"}, "Healthcheck":{"Test":["CMD","wget","http://localhost:8800/health"]}},
      "HostConfig":{"Binds":["config-file:/data/config.yaml:ro"], "Mounts":[{"Type":"volume","Source":"auth-data","Target":"/auth","VolumeOptions":{"NoCopy":true}}], "PortBindings":{"8800/tcp":[{"HostIp":"127.0.0.1","HostPort":"9900"}]}, "RestartPolicy":{"Name":"unless-stopped"}, "NetworkMode":"project_default", "AutoRemove":false},
      "Mounts":[{"Type":"volume","Name":"database","Destination":"DATA_DIR","RW":true},{"Type":"volume","Name":"anonymous-cookies","Destination":"/cookies","RW":true},{"Type":"volume","Name":"auth-data","Destination":"/auth","RW":true}],
      "NetworkSettings":{"Networks":{"project_default":{"Aliases":["web2api","old-containe"],"IPAddress":"10.0.0.2","EndpointID":"readonly","NetworkID":"network-id"}}},
      "State":{"Running":true,"Status":"running","Health":{"Status":"healthy"}}
    }`
	if err := json.Unmarshal([]byte(fixture), &old); err != nil {
		t.Fatal(err)
	}
	old.Mounts[0].Destination = dir
	fake := &fakeDocker{old: old, health: "healthy"}
	server := httptest.NewServer(http.HandlerFunc(fake.handle))
	t.Cleanup(server.Close)
	client := &http.Client{Transport: rewriteTransport{base: server.URL}}
	return &Manager{docker: &dockerClient{http: client}, enabled: true, image: "ghcr.io/plunjoin/web2api:latest", container: old.ID, socket: "/var/run/docker.sock", statePath: dir + "/upgrade-state.json"}, fake
}

type rewriteTransport struct{ base string }

func (r rewriteTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.URL.Scheme = "http"
	clone.URL.Host = strings.TrimPrefix(r.base, "http://")
	return http.DefaultTransport.RoundTrip(clone)
}

func (f *fakeDocker) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Method+" "+r.URL.RequestURI())
	jsonOut := func(v any) { w.Header().Set("Content-Type", "application/json"); _ = json.NewEncoder(w).Encode(v) }
	switch {
	case r.URL.Path == "/images/create":
		if f.pruneUntagged && !f.currentPinned {
			f.oldImageRemoved = true
		}
		if f.pullError {
			jsonOut(map[string]string{"error": "denied: permission_denied: read_package"})
		} else {
			jsonOut(map[string]string{"status": "Download complete"})
		}
	case r.URL.Path == "/images/sha256:old/tag":
		f.currentPinned = true
		w.WriteHeader(201)
	case r.URL.Path == "/images/sha256:old/json" && f.oldImageRemoved:
		w.WriteHeader(404)
		jsonOut(map[string]string{"message": "No such image: sha256:old"})
	case strings.HasPrefix(r.URL.Path, "/images/"):
		id := "sha256:new"
		if strings.Contains(r.URL.Path, "sha256:old") {
			id = "sha256:old"
		}
		jsonOut(map[string]any{"Id": id, "Config": map[string]any{"Labels": map[string]string{"org.opencontainers.image.version": "v0.2"}}})
	case r.URL.Path == "/containers/old-container-id/json":
		jsonOut(f.old)
	case strings.HasPrefix(r.URL.Path, "/containers/web2api-upgrade-") && strings.HasSuffix(r.URL.Path, "/json"):
		w.WriteHeader(404)
		jsonOut(map[string]string{"message": "No such container"})
	case r.URL.Path == "/containers/new-container/json":
		jsonOut(map[string]any{"State": map[string]any{"Running": true, "Health": map[string]string{"Status": f.health}}})
	case r.URL.Path == "/containers/create":
		var config map[string]any
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			w.WriteHeader(400)
			return
		}
		if strings.HasPrefix(r.URL.Query().Get("name"), "web2api-upgrade-") {
			if f.oldImageRemoved && config["Image"] == "sha256:old" {
				w.WriteHeader(404)
				jsonOut(map[string]string{"message": "No such image: sha256:old"})
				return
			}
			f.helper = config
			jsonOut(map[string]string{"Id": "helper"})
		} else {
			f.replacement = config
			jsonOut(map[string]string{"Id": "new-container"})
		}
	case r.URL.Path == "/containers/new-container/start" && f.failStart:
		w.WriteHeader(500)
		jsonOut(map[string]string{"message": "port allocation failed"})
	default:
		w.WriteHeader(204)
	}
}

func waitChecked(t *testing.T, m *Manager) State {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		state, err := m.load()
		m.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		if state.Phase != "checking" {
			return state
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("check did not complete")
	return State{}
}

func TestCheckDoesNotStopContainerAndStartsPinnedHelper(t *testing.T) {
	m, fake := newFakeDocker(t)
	if err := m.Check(); err != nil {
		t.Fatal(err)
	}
	state := waitChecked(t, m)
	if state.Phase != "ready" || state.TargetID != "sha256:new" {
		t.Fatalf("state: %+v", state)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background()); err != ErrConflict {
		t.Fatalf("duplicate start: %v", err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, request := range fake.requests {
		if strings.Contains(request, "/stop") {
			t.Fatal("main process stopped itself")
		}
	}
	if fake.helper["Image"] != "sha256:old" {
		t.Fatalf("helper must use current trusted image: %v", fake.helper)
	}
	cmd, _ := json.Marshal(fake.helper["Cmd"])
	if !strings.Contains(string(cmd), "sha256:new") || strings.Contains(string(cmd), "latest") {
		t.Fatalf("unpinned helper: %s", cmd)
	}
	host := fake.helper["HostConfig"].(map[string]any)
	if host["NetworkMode"] != "none" {
		t.Fatal("helper should use only the local Docker socket")
	}
}

func TestPullStreamFailureLeavesOldContainerRunning(t *testing.T) {
	m, fake := newFakeDocker(t)
	fake.pullError = true
	if err := m.Check(); err != nil {
		t.Fatal(err)
	}
	state := waitChecked(t, m)
	if state.Phase != "failed" || !strings.Contains(state.Message, "read_package") {
		t.Fatalf("state: %+v", state)
	}
	if err := m.Start(context.Background()); err != ErrConflict {
		t.Fatal(err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, request := range fake.requests {
		if strings.Contains(request, "/stop") {
			t.Fatal("pull failure must not stop service")
		}
	}
}

func TestCurrentImagePinnedBeforeLatestMoves(t *testing.T) {
	m, fake := newFakeDocker(t)
	fake.pruneUntagged = true
	if err := m.Check(); err != nil {
		t.Fatal(err)
	}
	if state := waitChecked(t, m); state.Phase != "ready" {
		t.Fatalf("check: %+v", state)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("moving latest must not discard the current helper image: %v", err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	pin, pull := -1, -1
	for index, request := range fake.requests {
		if strings.Contains(request, "/images/sha256:old/tag?repo=web2api-upgrade-cache&tag=old") {
			pin = index
		}
		if strings.Contains(request, "/images/create?") {
			pull = index
		}
	}
	if pin < 0 || pull <= pin {
		t.Fatalf("current image must stay addressable after latest is pulled: %v", fake.requests)
	}
}

func prepareHelper(t *testing.T, m *Manager) {
	t.Helper()
	if err := m.save(State{JobID: "job", TargetID: "sha256:new", Phase: "starting"}); err != nil {
		t.Fatal(err)
	}
}

func TestReplacementPreservesDeploymentAndVolumes(t *testing.T) {
	t.Parallel()
	m, fake := newFakeDocker(t)
	prepareHelper(t, m)
	if err := m.replace("old-container-id", "sha256:new", "job", time.Second); err != nil {
		t.Fatal(err)
	}
	state, err := m.load()
	if err != nil || state.Phase != "completed" {
		t.Fatalf("%+v %v", state, err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	config := fake.replacement
	if config["Image"] != "sha256:new" || fmt.Sprint(config["Env"]) != "[CUSTOM=value]" {
		t.Fatalf("config: %v", config)
	}
	labels := config["Labels"].(map[string]any)
	if labels["com.docker.compose.service"] != "web2api" || labels["org.opencontainers.image.version"] != "v0.2" {
		t.Fatalf("labels: %v", labels)
	}
	host := config["HostConfig"].(map[string]any)
	binds := fmt.Sprint(host["Binds"])
	if !strings.Contains(binds, "anonymous-cookies:/cookies:rw") || !strings.Contains(binds, "database:"+fake.old.Mounts[0].Destination+":rw") || !strings.Contains(binds, "config-file:/data/config.yaml:ro") {
		t.Fatalf("lost existing mounts: %v", host)
	}
	if strings.Count(binds, "auth-data") > 0 {
		t.Fatal("mount defined through HostConfig.Mounts must not be duplicated")
	}
	if host["PortBindings"] == nil || host["RestartPolicy"] == nil {
		t.Fatal("deployment settings lost")
	}
	network := config["NetworkingConfig"].(map[string]any)["EndpointsConfig"].(map[string]any)["project_default"].(map[string]any)
	if network["IPAddress"] != nil || network["EndpointID"] != nil || fmt.Sprint(network["Aliases"]) != "[web2api]" {
		t.Fatalf("bad network recreation: %v", network)
	}
	last := fake.requests[len(fake.requests)-1]
	if last != "DELETE /containers/old-container-id?force=true&v=false" {
		t.Fatalf("cleanup must preserve volumes: %s", last)
	}
}

func TestFailedUpgradeRollsBackOldContainer(t *testing.T) {
	for _, failure := range []string{"start", "health"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			m, fake := newFakeDocker(t)
			prepareHelper(t, m)
			if failure == "start" {
				fake.failStart = true
			} else {
				fake.health = "unhealthy"
			}
			if err := m.replace("old-container-id", "sha256:new", "job", time.Second); err == nil {
				t.Fatal("expected upgrade failure")
			}
			state, err := m.load()
			if err != nil || state.Phase != "rolled_back" {
				t.Fatalf("%+v %v", state, err)
			}
			fake.mu.Lock()
			defer fake.mu.Unlock()
			requests := strings.Join(fake.requests, "\n")
			if strings.Contains(requests, "DELETE /containers/old-container-id") {
				t.Fatal("rollback deleted backup")
			}
			if !strings.Contains(requests, "DELETE /containers/new-container?force=true&v=false") || !strings.Contains(requests, "POST /containers/old-container-id/rename?name=web2api") || !strings.Contains(requests, "POST /containers/old-container-id/start") || !strings.HasSuffix(requests, "GET /containers/old-container-id/json") {
				t.Fatalf("rollback incomplete: %s", requests)
			}
		})
	}
}

func TestUnsafeDeploymentRefusesUpgrade(t *testing.T) {
	m, fake := newFakeDocker(t)
	fake.old.HostConfig["AutoRemove"] = true
	if err := m.save(State{Phase: "ready", TargetID: "sha256:new"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background()); err == nil {
		t.Fatal("--rm deployment cannot retain a backup")
	}
}

func TestInterruptedHelperDoesNotLockPanel(t *testing.T) {
	for _, targetRunning := range []bool{false, true} {
		t.Run(fmt.Sprint(targetRunning), func(t *testing.T) {
			m, fake := newFakeDocker(t)
			if targetRunning {
				fake.old.Image = "sha256:new"
			}
			state := State{Phase: "upgrading", JobID: "gone", TargetID: "sha256:new", UpdatedAt: time.Now().Add(-time.Minute)}
			if err := saveState(m.statePath, state); err != nil {
				t.Fatal(err)
			}
			status := m.Status(context.Background())
			want := "failed"
			if targetRunning {
				want = "completed"
			}
			if status.Phase != want {
				t.Fatalf("state: %+v", status)
			}
		})
	}
}
