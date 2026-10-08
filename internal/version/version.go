// Package version 提供构建版本信息。
//
// 发布构建通过 -ldflags "-X web2api/internal/version.Version=v0.2.5" 注入版本号；
// 本地 go build 时回退到 Go 自动嵌入的 VCS 信息（提交号与是否有未提交改动）。
package version

import (
	"runtime"
	"runtime/debug"
	"sync"
)

// Version 由 -ldflags -X 注入；未注入时为 "dev"。
var Version = "dev"

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
		cached = Info{Version: Version, GoVersion: runtime.Version()}
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
