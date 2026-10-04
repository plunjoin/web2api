package upgrade

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// RunHelper runs in a detached container, so stopping the web server cannot
// interrupt replacement or rollback. It never deletes data volumes.
func RunHelper(socket, container, target, job, statePath string) error {
	manager := &Manager{docker: newDockerClient(socket), statePath: statePath}
	return manager.replace(container, target, job, 3*time.Minute)
}

func (m *Manager) replace(container, target, job string, healthTimeout time.Duration) (result error) {
	state, err := m.load()
	if err != nil {
		return err
	}
	if state.JobID != job || state.TargetID != target || state.Phase != "starting" {
		return errors.New("升级任务记录不匹配")
	}
	write := func(phase, message string) { state.Phase, state.Message = phase, message; _ = m.save(state) }
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	old, err := m.docker.inspect(ctx, container)
	if err != nil {
		write("failed", err.Error())
		return err
	}
	if err := validateContainer(old, m.statePath); err != nil {
		write("failed", err.Error())
		return err
	}
	image, err := m.docker.image(ctx, target)
	if err != nil {
		write("failed", err.Error())
		return err
	}
	name := strings.TrimPrefix(old.Name, "/")
	if name == "" || !old.State.Running {
		err := errors.New("原容器未运行或没有名称")
		write("failed", err.Error())
		return err
	}
	var replacement string
	renamed, stopped := false, false
	// A failure after stopping the old container always attempts rollback
	// using a new context, even if the original operation timed out.
	defer func() {
		if result == nil {
			return
		}
		if !stopped && !renamed {
			write("failed", result.Error())
			return
		}
		recoverCtx, recoverCancel := context.WithTimeout(context.Background(), healthTimeout+time.Minute)
		defer recoverCancel()
		var rollbackErrors []error
		if replacement != "" {
			if err := m.docker.remove(recoverCtx, replacement); err != nil {
				rollbackErrors = append(rollbackErrors, err)
			}
		}
		if renamed {
			if err := m.docker.rename(recoverCtx, old.ID, name); err != nil {
				rollbackErrors = append(rollbackErrors, err)
			}
		}
		if err := m.docker.start(recoverCtx, old.ID); err != nil {
			rollbackErrors = append(rollbackErrors, err)
		} else if err := m.waitHealthy(recoverCtx, old.ID, healthTimeout); err != nil {
			rollbackErrors = append(rollbackErrors, err)
		}
		if len(rollbackErrors) > 0 {
			write("failed", fmt.Sprintf("升级失败: %v；恢复旧容器失败: %v", result, errors.Join(rollbackErrors...)))
		} else {
			write("rolled_back", "升级失败，已恢复旧容器: "+result.Error())
		}
	}()
	write("upgrading", "正在切换容器，配置和数据卷保持原有挂载。")
	// Delay gives the initiating HTTP response time to reach the browser.
	timer := time.NewTimer(2 * time.Second)
	select {
	case <-ctx.Done():
		timer.Stop()
		return ctx.Err()
	case <-timer.C:
	}
	if err := m.docker.request(ctx, "POST", "/containers/"+old.ID+"/stop?t=20", nil, nil); err != nil {
		// A timed-out stop can still have stopped the daemon-side container.
		stopped = true
		return err
	}
	stopped = true
	if err := m.docker.rename(ctx, old.ID, name+"-backup-"+job); err != nil {
		return err
	}
	renamed = true
	replacement, err = m.docker.create(ctx, name, replacementConfig(old, image))
	if err != nil {
		return err
	}
	if err := m.docker.start(ctx, replacement); err != nil {
		return err
	}
	write("restarting", "新容器已启动，正在等待健康检查。")
	if err := m.waitHealthy(ctx, replacement, healthTimeout); err != nil {
		return err
	}
	// Commit replacement before cleanup; failure to clean up a stopped backup
	// must not revert an otherwise healthy deployment.
	write("completed", "升级完成，新容器已通过健康检查。")
	if err := m.docker.remove(ctx, old.ID); err != nil {
		write("completed", "升级完成；旧容器备份清理失败，可稍后手动清理。")
	}
	return nil
}

func (m *Manager) waitHealthy(ctx context.Context, id string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		info, err := m.docker.inspect(ctx, id)
		if err != nil {
			return err
		}
		if !info.State.Running {
			return errors.New("新容器已退出")
		}
		if info.State.Health == nil {
			return errors.New("新容器缺少 Docker 健康检查，请启用部署模板中的 healthcheck")
		}
		if info.State.Health.Status == "healthy" {
			return nil
		}
		if info.State.Health.Status == "unhealthy" {
			return errors.New("新容器健康检查失败")
		}
		select {
		case <-ctx.Done():
			return errors.New("等待新容器健康检查超时")
		case <-ticker.C:
		}
	}
}
