// Package upgrade updates a Docker deployment through a detached helper.
package upgrade

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

type dockerClient struct {
	http *http.Client
}

type dockerError struct {
	code    int
	message string
}

func (e *dockerError) Error() string { return fmt.Sprintf("Docker HTTP %d: %s", e.code, e.message) }

func newDockerClient(socket string) *dockerClient {
	return &dockerClient{http: &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		},
	}}}
}

func (d *dockerClient) request(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://docker"+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.http.Do(req)
	if err != nil {
		return fmt.Errorf("连接 Docker 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var problem struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&problem)
		return &dockerError{code: resp.StatusCode, message: problem.Message}
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out)
	}
	_, err = io.Copy(io.Discard, resp.Body)
	return err
}

type containerInfo struct {
	ID         string `json:"Id"`
	Name       string
	Image      string
	Config     map[string]any
	HostConfig map[string]any
	Mounts     []struct {
		Type, Name, Source, Destination string
		RW                              bool
	}
	NetworkSettings struct{ Networks map[string]json.RawMessage }
	State           struct {
		Running bool
		Status  string
		Health  *struct{ Status string }
	}
}

type imageInfo struct {
	ID          string `json:"Id"`
	RepoDigests []string
	Config      struct{ Labels map[string]string }
}

func (d *dockerClient) inspect(ctx context.Context, id string) (containerInfo, error) {
	var info containerInfo
	err := d.request(ctx, "GET", "/containers/"+url.PathEscape(id)+"/json", nil, &info)
	return info, err
}

func (d *dockerClient) image(ctx context.Context, id string) (imageInfo, error) {
	var info imageInfo
	err := d.request(ctx, "GET", "/images/"+url.PathEscape(id)+"/json", nil, &info)
	return info, err
}

func (d *dockerClient) pin(ctx context.Context, id string) error {
	// Docker Desktop's containerd store can discard an untagged image index
	// when latest moves, even while a container still uses its filesystem.
	// Keep an explicit local reference for the detached helper and rollback.
	return d.request(ctx, "POST", "/images/"+url.PathEscape(id)+"/tag?repo=web2api-upgrade-cache&tag="+url.QueryEscape(strings.TrimPrefix(id, "sha256:")), nil, nil)
}

func (d *dockerClient) pull(ctx context.Context, image, registryAuth string) error {
	req, err := http.NewRequestWithContext(ctx, "POST", "http://docker/images/create?"+imageCreateQuery(image), nil)
	if err != nil {
		return err
	}
	if registryAuth != "" {
		req.Header.Set("X-Registry-Auth", registryAuth)
	}
	resp, err := d.http.Do(req)
	if err != nil {
		return fmt.Errorf("拉取镜像失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		message := strings.TrimSpace(string(body))
		var problem struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(body, &problem) == nil && problem.Message != "" {
			message = problem.Message
		}
		if message == "" {
			message = "请检查镜像地址及仓库访问权限"
		}
		return fmt.Errorf("拉取镜像失败（HTTP %d）: %s", resp.StatusCode, message)
	}
	decoder := json.NewDecoder(resp.Body)
	for {
		var event struct {
			Error       string `json:"error"`
			ErrorDetail struct {
				Message string `json:"message"`
			} `json:"errorDetail"`
		}
		err := decoder.Decode(&event)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("读取镜像下载状态失败: %w", err)
		}
		message := event.Error
		if message == "" {
			message = event.ErrorDetail.Message
		}
		if message != "" {
			return fmt.Errorf("拉取镜像失败: %s", message)
		}
	}
}

func (d *dockerClient) create(ctx context.Context, name string, config map[string]any) (string, error) {
	var result struct {
		ID string `json:"Id"`
	}
	err := d.request(ctx, "POST", "/containers/create?name="+url.QueryEscape(name), config, &result)
	if err == nil && result.ID == "" {
		err = fmt.Errorf("Docker 未返回新容器 ID")
	}
	return result.ID, err
}

func (d *dockerClient) start(ctx context.Context, id string) error {
	return d.request(ctx, "POST", "/containers/"+url.PathEscape(id)+"/start", nil, nil)
}

func (d *dockerClient) rename(ctx context.Context, id, name string) error {
	return d.request(ctx, "POST", "/containers/"+url.PathEscape(id)+"/rename?name="+url.QueryEscape(strings.TrimPrefix(name, "/")), nil, nil)
}

func (d *dockerClient) remove(ctx context.Context, id string) error {
	// Never delete persistent/anonymous volumes.
	return d.request(ctx, "DELETE", "/containers/"+url.PathEscape(id)+"?force=true&v=false", nil, nil)
}

func replacementConfig(old containerInfo, image imageInfo) map[string]any {
	config := old.Config
	config["Image"] = image.ID
	if hostname, _ := config["Hostname"].(string); strings.HasPrefix(old.ID, hostname) && hostname != "" {
		config["Hostname"] = ""
	}
	// Preserve deployment labels, while refreshing image version metadata.
	labels, _ := config["Labels"].(map[string]any)
	if labels == nil {
		labels = map[string]any{}
	}
	for key := range labels {
		if strings.HasPrefix(key, "org.opencontainers.image.") {
			delete(labels, key)
		}
	}
	for key, value := range image.Config.Labels {
		if strings.HasPrefix(key, "org.opencontainers.image.") {
			labels[key] = value
		}
	}
	config["Labels"] = labels
	host := old.HostConfig
	// Docker assigns anonymous volume names at creation. Pin those names so
	// a replacement reuses the same data rather than creating empty volumes.
	bound := map[string]bool{}
	binds, _ := host["Binds"].([]any)
	for _, value := range binds {
		parts := strings.Split(fmt.Sprint(value), ":")
		if len(parts) > 1 {
			bound[parts[1]] = true
		}
	}
	if mounts, ok := host["Mounts"].([]any); ok {
		for _, value := range mounts {
			if mount, ok := value.(map[string]any); ok {
				bound[fmt.Sprint(mount["Target"])] = true
			}
		}
	}
	for _, mount := range old.Mounts {
		if mount.Type == "volume" && !bound[mount.Destination] {
			mode := "rw"
			if !mount.RW {
				mode = "ro"
			}
			binds = append(binds, mount.Name+":"+mount.Destination+":"+mode)
		}
	}
	host["Binds"] = binds
	config["HostConfig"] = host
	endpoints := map[string]any{}
	for name, raw := range old.NetworkSettings.Networks {
		var endpoint map[string]any
		_ = json.Unmarshal(raw, &endpoint)
		clean := map[string]any{}
		for _, key := range []string{"Aliases", "IPAMConfig", "DriverOpts", "GwPriority"} {
			if value, ok := endpoint[key]; ok {
				clean[key] = value
			}
		}
		if aliases, ok := clean["Aliases"].([]any); ok {
			filtered := []any{}
			for _, alias := range aliases {
				if value := fmt.Sprint(alias); value != old.ID && value != old.ID[:min(12, len(old.ID))] {
					filtered = append(filtered, alias)
				}
			}
			clean["Aliases"] = filtered
		}
		endpoints[name] = clean
	}
	config["NetworkingConfig"] = map[string]any{"EndpointsConfig": endpoints}
	return config
}
