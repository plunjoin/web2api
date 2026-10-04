package upgrade

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

var ErrConflict = errors.New("升级任务正在运行或尚未检查到可用更新")

type State struct {
	JobID         string    `json:"job_id,omitempty"`
	Phase         string    `json:"phase"`
	Message       string    `json:"message"`
	TargetID      string    `json:"target_id,omitempty"`
	TargetVersion string    `json:"target_version,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Status struct {
	State
	Enabled        bool   `json:"enabled"`
	Reason         string `json:"reason,omitempty"`
	Image          string `json:"image"`
	CurrentID      string `json:"current_id,omitempty"`
	CurrentVersion string `json:"current_version,omitempty"`
}

type Manager struct {
	mu                                                        sync.Mutex
	docker                                                    *dockerClient
	enabled                                                   bool
	reason, socket, container, image, statePath, registryAuth string
}

func Disabled() *Manager {
	return &Manager{image: "ghcr.io/plunjoin/web2api:latest", reason: "当前部署未启用面板升级，请先按部署文档启用 Docker 升级配置。"}
}

// NewFromEnv is opt-in; only trusted deployment configuration selects an image.
func NewFromEnv(dataDir string) *Manager {
	m := Disabled()
	if os.Getenv("WEB2API_UPGRADE_ENABLED") != "true" {
		return m
	}
	if runtime.GOOS != "linux" {
		m.reason = "面板镜像升级适用于 Linux Docker 容器。"
		return m
	}
	m.socket = envOr("WEB2API_UPGRADE_SOCKET", "/var/run/docker.sock")
	if _, err := os.Stat(m.socket); err != nil {
		m.reason = "Docker socket 未挂载，请按部署文档启用升级配置。"
		return m
	}
	m.container = envOr("WEB2API_UPGRADE_CONTAINER", os.Getenv("HOSTNAME"))
	if m.container == "" {
		m.reason = "无法识别当前容器，请配置 WEB2API_UPGRADE_CONTAINER。"
		return m
	}
	absoluteDir, err := filepath.Abs(dataDir)
	if err != nil {
		m.reason = "无法定位升级记录目录: " + err.Error()
		return m
	}
	m.enabled, m.reason = true, ""
	m.image = envOr("WEB2API_UPGRADE_IMAGE", m.image)
	m.statePath = filepath.Join(absoluteDir, "upgrade-state.json")
	m.docker = newDockerClient(m.socket)
	if password := os.Getenv("WEB2API_UPGRADE_REGISTRY_PASSWORD"); password != "" {
		credentials, _ := json.Marshal(map[string]string{"username": os.Getenv("WEB2API_UPGRADE_REGISTRY_USER"), "password": password})
		m.registryAuth = base64.URLEncoding.EncodeToString(credentials)
	}
	// A pull runs inside the main process; a replacement runs independently.
	if state, err := m.load(); err == nil && state.Phase == "checking" {
		state.Phase, state.Message = "failed", "上次镜像检查被服务重启中断，请重新检查。"
		_ = m.save(state)
	}
	return m
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func (m *Manager) load() (State, error) {
	state := State{Phase: "idle", Message: "点击检查更新，获取最新镜像。"}
	data, err := os.ReadFile(m.statePath)
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	err = json.Unmarshal(data, &state)
	return state, err
}

func (m *Manager) save(state State) error {
	state.UpdatedAt = time.Now().UTC()
	return saveState(m.statePath, state)
}

func saveState(path string, state State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".upgrade-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func busy(phase string) bool {
	return phase == "checking" || phase == "starting" || phase == "upgrading" || phase == "restarting"
}

func (m *Manager) Status(ctx context.Context) Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := State{Phase: "idle", Message: m.reason}
	if m.enabled {
		var err error
		state, err = m.load()
		if err != nil {
			state.Phase, state.Message = "failed", "读取升级记录失败: "+err.Error()
		}
	}
	status := Status{State: state, Enabled: m.enabled, Reason: m.reason, Image: m.image}
	if !m.enabled {
		return status
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	current, err := m.docker.inspect(ctx, m.container)
	if err != nil {
		status.Enabled, status.Reason = false, err.Error()
		return status
	}
	status.CurrentID = current.Image
	// A crashed/removed helper must not leave the panel permanently locked.
	if busy(state.Phase) && state.Phase != "checking" && state.JobID != "" && time.Since(state.UpdatedAt) > 10*time.Second {
		helper, helperErr := m.docker.inspect(ctx, "web2api-upgrade-"+state.JobID)
		var apiError *dockerError
		missing := errors.As(helperErr, &apiError) && apiError.code == 404
		if missing || (helperErr == nil && !helper.State.Running) {
			state.Phase, state.Message = "failed", "升级任务已中断，当前服务已恢复连接，请重新检查镜像和容器状态。"
			if current.Image == state.TargetID && current.State.Running && current.State.Health != nil && current.State.Health.Status == "healthy" {
				state.Phase, state.Message = "completed", "升级完成，当前容器已通过健康检查。"
			}
			_ = m.save(state)
			status.State = state
		}
	}
	if image, err := m.docker.image(ctx, current.Image); err == nil {
		status.CurrentVersion = image.Config.Labels["org.opencontainers.image.version"]
	}
	return status
}

func (m *Manager) Check() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.enabled {
		return errors.New(m.reason)
	}
	state, err := m.load()
	if err != nil {
		return err
	}
	if busy(state.Phase) {
		return ErrConflict
	}
	state = State{Phase: "checking", Message: "正在拉取最新镜像，服务继续运行。"}
	if err := m.save(state); err != nil {
		return err
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		err := m.docker.pull(ctx, m.image, m.registryAuth)
		var latest imageInfo
		var current containerInfo
		if err == nil {
			latest, err = m.docker.image(ctx, m.image)
		}
		if err == nil {
			current, err = m.docker.inspect(ctx, m.container)
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if err != nil {
			state.Phase, state.Message = "failed", err.Error()
		} else {
			state.TargetID, state.TargetVersion = latest.ID, latest.Config.Labels["org.opencontainers.image.version"]
			state.Phase, state.Message = "ready", "最新镜像已就绪，可以开始升级。"
			if latest.ID == current.Image {
				state.Phase, state.Message = "up_to_date", "当前已是最新镜像。"
			}
		}
		_ = m.save(state)
	}()
	return nil
}

func validateContainer(current containerInfo, statePath string) error {
	if current.Config == nil || current.HostConfig == nil || current.State.Health == nil {
		return errors.New("当前容器缺少有效的 Docker 健康检查，请先启用部署模板中的 healthcheck。")
	}
	if auto, _ := current.HostConfig["AutoRemove"].(bool); auto {
		return errors.New("不支持 --rm 容器升级，请使用持久化的 Docker Compose 部署。")
	}
	if mode, _ := current.HostConfig["NetworkMode"].(string); strings.HasPrefix(mode, "container:") {
		return errors.New("不支持共享其他容器网络的部署。")
	}
	if volumes, _ := current.HostConfig["VolumesFrom"].([]any); len(volumes) > 0 {
		return errors.New("请将 volumes_from 改为明确的持久化卷挂载后启用升级。")
	}
	// The helper must write status into the same mounted data directory.
	for _, mount := range current.Mounts {
		if mount.RW && (mount.Type == "bind" || mount.Type == "volume") && (statePath == mount.Destination || strings.HasPrefix(statePath, strings.TrimRight(mount.Destination, "/")+"/")) {
			return nil
		}
	}
	return errors.New("升级记录目录必须挂载可写的持久化数据卷。")
}

func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.enabled {
		return errors.New(m.reason)
	}
	state, err := m.load()
	if err != nil {
		return err
	}
	if state.Phase != "ready" || !strings.HasPrefix(state.TargetID, "sha256:") {
		return ErrConflict
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	current, err := m.docker.inspect(ctx, m.container)
	if err != nil {
		return err
	}
	if err := validateContainer(current, m.statePath); err != nil {
		return err
	}
	if current.Image == state.TargetID {
		return ErrConflict
	}
	// The helper uses the running image, and replaces with the already checked
	// immutable image ID. A changing latest tag cannot change this job's target.
	if _, err := m.docker.image(ctx, state.TargetID); err != nil {
		return err
	}
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return err
	}
	state.JobID = hex.EncodeToString(random)
	state.Phase, state.Message = "starting", "升级任务已启动，重启期间面板会自动重新连接。"
	if err := m.save(state); err != nil {
		return err
	}
	helper, err := m.docker.create(ctx, "web2api-upgrade-"+state.JobID, map[string]any{
		"Image":       current.Image,
		"Entrypoint":  []string{"/app/web2api"},
		"Cmd":         []string{"-upgrade-container", current.ID, "-upgrade-image", state.TargetID, "-upgrade-job", state.JobID, "-upgrade-state", m.statePath, "-upgrade-socket", m.socket},
		"User":        current.Config["User"],
		"Labels":      map[string]string{"web2api.upgrade.job": state.JobID},
		"Healthcheck": map[string]any{"Test": []string{"NONE"}},
		"HostConfig":  map[string]any{"AutoRemove": true, "NetworkMode": "none", "VolumesFrom": []string{current.ID}, "Binds": []string{m.socket + ":" + m.socket}},
	})
	if err == nil {
		err = m.docker.start(ctx, helper)
	}
	if err != nil {
		if helper != "" {
			cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_ = m.docker.remove(cleanup, helper)
			cancel()
		}
		state.Phase, state.Message = "failed", err.Error()
		_ = m.save(state)
		return fmt.Errorf("启动升级任务失败: %w", err)
	}
	return nil
}
