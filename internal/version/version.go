// Package version 提供构建版本信息。
//
// 发布构建通过 -ldflags "-X web2api/internal/version.Version=v0.2.6" 注入版本号；
// 未注入（或注入为 "dev"，例如 Docker 构建未传 VERSION）时回退到仓库内的 VERSION 文件。
// 提交号与是否有未提交改动取自 Go 自动嵌入的 VCS 信息。
package version

import (
	_ "embed"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
)

// Version 由 -ldflags -X 注入；未注入时为 "dev"。
var Version = "dev"

// releaseVersion 是随源码发布的版本号，发版前与 tag 一同更新。
//
//go:embed VERSION
var releaseVersion string

// resolveVersion 优先使用 ldflags 注入值，否则使用内嵌 VERSION 文件。
func resolveVersion(injected, embedded string) string {
	injected = strings.TrimSpace(injected)
	if injected != "" && injected != "dev" {
		return injected
	}
	if embedded = strings.TrimSpace(embedded); embedded != "" {
		return embedded
	}
	return "dev"
}

// Info 版本详情。
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit,omitempty"`
	BuildTime string `json:"build_time,omitempty"`
	Modified  bool   `json:"modified,omitempty"`
	GoVersion string `json:"go_version"`
}

var (
	once   sync.Once
	cached Info
)

// Get 返回版本详情（进程内缓存）。
func Get() Info {
	once.Do(func() {
		cached = Info{Version: resolveVersion(Version, releaseVersion), GoVersion: runtime.Version()}
		if build, ok := debug.ReadBuildInfo(); ok {
			for _, setting := range build.Settings {
				switch setting.Key {
				case "vcs.revision":
					cached.Commit = setting.Value
					if len(cached.Commit) > 12 {
						cached.Commit = cached.Commit[:12]
					}
				case "vcs.time":
					cached.BuildTime = setting.Value
				case "vcs.modified":
					cached.Modified = setting.Value == "true"
				}
			}
		}
	})
	return cached
}
