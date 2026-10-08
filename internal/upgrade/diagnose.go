package upgrade

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// 拉取被拒（401/403/denied）时，程序直接匿名访问镜像仓库，判断问题出在哪一侧，
// 并给出需要用户提供或修改的具体设置。GHCR 实测行为（2026-10）：
//   - 公开包：匿名 GET /token → 200，manifest → 200；
//   - 私有包或镜像名错误：匿名 GET /token → 403 DENIED（两者无法区分）；
//   - 携带无效 Basic 凭据（过期/无 read:packages 的 PAT、fine-grained PAT、只填用户名）→ 403 DENIED。

type anonymousProbe struct {
	reachable bool // 容器内能访问仓库
	readable  bool // 匿名可读取 manifest（公开镜像）
	status    int  // 拒绝时的 HTTP 状态
	detail    string
}

func manifestReference(ref imageRef) string {
	switch {
	case ref.digest != "":
		return ref.digest
	case ref.tag != "":
		return ref.tag
	default:
		return "latest"
	}
}

// probeAnonymous 匿名申请拉取令牌并 HEAD manifest。
func (m *Manager) probeAnonymous(ctx context.Context, ref imageRef) anonymousProbe {
	if m.registryHTTP == nil {
		return anonymousProbe{detail: "未配置仓库客户端"}
	}
	token, status, err := m.requestBearer(ctx, ref, "", "")
	if err != nil && status == 0 {
		return anonymousProbe{detail: err.Error()}
	}
	if err != nil && status != http.StatusOK {
		// 令牌接口拒绝匿名访问：私有包或镜像名错误。
		return anonymousProbe{reachable: true, status: status, detail: err.Error()}
	}
	base := strings.TrimSuffix(registryPingURL(ref.registry), "/v2/")
	req, reqErr := http.NewRequestWithContext(ctx, http.MethodHead, base+"/v2/"+ref.repository+"/manifests/"+manifestReference(ref), nil)
	if reqErr != nil {
		return anonymousProbe{detail: reqErr.Error()}
	}
	req.Header.Set("Accept", strings.Join([]string{
		"application/vnd.oci.image.index.v1+json",
		"application/vnd.docker.distribution.manifest.list.v2+json",
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
	}, ", "))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, doErr := m.registryHTTP.Do(req)
	if doErr != nil {
		return anonymousProbe{detail: doErr.Error()}
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return anonymousProbe{reachable: true, readable: true}
	}
	return anonymousProbe{reachable: true, status: resp.StatusCode, detail: fmt.Sprintf("manifest 返回 HTTP %d", resp.StatusCode)}
}

// credentialHint 说明需要配置的凭据变量及取值要求。
func credentialHint(registry string) string {
	if registry == "ghcr.io" {
		return "WEB2API_UPGRADE_REGISTRY_USER=<GitHub 用户名> 和 WEB2API_UPGRADE_REGISTRY_PASSWORD=<具有 read:packages 权限的 GitHub classic Personal Access Token；GHCR 不接受 fine-grained Token 或 GitHub 登录密码>"
	}
	return "WEB2API_UPGRADE_REGISTRY_USER 和 WEB2API_UPGRADE_REGISTRY_PASSWORD（" + registry + " 的用户名与访问令牌/密码）"
}

// explainPullError 在拉取被拒时补充诊断结论；其他错误原样返回。
func (m *Manager) explainPullError(ctx context.Context, err error) error {
	if err == nil || !registryDenied(err) {
		return err
	}
	ref, parseErr := parseImageRef(m.image)
	if parseErr != nil {
		return fmt.Errorf("%v。诊断：WEB2API_UPGRADE_IMAGE=%q 无法解析，请改为完整镜像地址（如 ghcr.io/<owner>/<name>:latest）", err, m.image)
	}
	credsConfigured := m.registryPassword != "" || m.registryUser != ""
	// 实际用配置的凭据申请一次令牌，区分“凭据被拒”与“凭据有效”。
	credsRejected := false
	if m.registryPassword != "" && m.registryHTTP != nil {
		if _, status, credErr := m.requestBearer(ctx, ref, m.registryUser, m.registryPassword); credErr != nil &&
			(status == http.StatusUnauthorized || status == http.StatusForbidden) {
			credsRejected = true
		}
	}
	recreate := "修改 .env 后执行 docker compose -f docker-compose.yml -f docker-compose.upgrade.yml up -d 让新设置生效。"
	probe := m.probeAnonymous(ctx, ref)
	var hint string
	switch {
	case !probe.reachable:
		hint = fmt.Sprintf("诊断：容器内无法访问 %s（%s），无法确认镜像是否公开。若服务器访问 %s 需要代理，请在 .env 设置 WEB2API_PROXY，并为宿主机 Docker 守护进程配置同样的代理（Docker 拉取由宿主机守护进程执行）；若 %s 是私有镜像，请设置 %s。%s",
			ref.registry, probe.detail, ref.registry, m.image, credentialHint(ref.registry), recreate)
	case probe.readable:
		hint = fmt.Sprintf("诊断：%s 是公开镜像，本程序已匿名读取成功，不需要任何仓库凭据；程序也已改用匿名方式重试，宿主机 Docker 守护进程仍被拒绝。", m.image)
		if credsRejected {
			hint += "已配置的 WEB2API_UPGRADE_REGISTRY_USER/WEB2API_UPGRADE_REGISTRY_PASSWORD 被仓库拒绝（HTTP 403），公开镜像无需凭据，建议清空这两个变量。"
		}
		hint += "请检查宿主机 Docker 的出站代理或镜像加速（/etc/docker/daemon.json、systemd 中 dockerd 的 HTTPS_PROXY）、Docker Desktop 的 Registry Access Management，以及 Docker socket 是否经过会拒绝 /images/create 的代理或授权插件；可在宿主机执行 docker pull " + m.image + " 复现。" + recreate
	default:
		hint = fmt.Sprintf("诊断：%s 无法匿名读取（仓库返回 HTTP %d）：镜像名错误或为私有包。请核对 WEB2API_UPGRADE_IMAGE（当前 %s）；若为私有包，请设置 %s。",
			ref.registry, probe.status, m.image, credentialHint(ref.registry))
		switch {
		case credsRejected:
			hint += "当前配置的凭据被仓库拒绝：请确认用户名与 Token 匹配、Token 未过期且有 read:packages 权限。"
		case credsConfigured && m.registryPassword == "":
			hint += "当前只设置了 WEB2API_UPGRADE_REGISTRY_USER，WEB2API_UPGRADE_REGISTRY_PASSWORD 为空，凭据不会生效。"
		case credsConfigured:
			hint += "当前凭据可以申请令牌，但无权读取该镜像：请确认 Token 所属账号对该包有读取权限。"
		}
		hint += recreate
	}
	return fmt.Errorf("%v。%s", err, hint)
}
