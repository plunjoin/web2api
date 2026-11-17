// Command web：web/ 前端的构建与开发入口（纯 Go，零 Node）。
//
//	go run ./tools/web build   # Tailwind CSS → wf build → internal/webui/dist
//	go run ./tools/web dev     # Tailwind --watch + wf dev（Go 改动自动重启、前端 HMR）
//	go run ./tools/web css     # 只重新生成 Tailwind CSS
//
// 前端依赖用 `go tool wf install .` 装到 web/wf_modules（读 package-lock.json）；
// Tailwind v4 没有 Go 实现，这里用官方 standalone 可执行文件（同样不需要 Node），
// 首次运行按需下载并缓存在 web/.tools/。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"

	"github.com/plunjoin/webfront/cli"
)

func main() {
	cmd := "build"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	root, err := repoRoot()
	if err != nil {
		fatal(err)
	}
	must(chdir(root))
	webDir := filepath.Join(root, "web")

	switch cmd {
	case "build":
		must(installDeps())
		exe := mustTailwind(webDir)
		must(genIcons(webDir))
		must(buildCSS(webDir, exe, true))
		dist := filepath.Join(root, "internal", "webui", "dist")
		must(os.RemoveAll(dist))
		must(os.MkdirAll(dist, 0o755))
		code := cli.Run([]string{"build", "-mode", "spa", "-base", "/admin",
			"-out", filepath.ToSlash(filepath.Join("internal", "webui", "dist")), "web"})
		if code != 0 {
			os.Exit(code)
		}
		must(copyIcons(webDir, dist))
		must(patchIndex(dist))
		fmt.Println("✅ web/ 构建完成 → internal/webui/dist")
	case "css":
		must(installDeps())
		exe := mustTailwind(webDir)
		must(genIcons(webDir))
		must(buildCSS(webDir, exe, true))
	case "dev":
		must(installDeps())
		exe := mustTailwind(webDir)
		must(genIcons(webDir))
		must(buildCSS(webDir, exe, false))
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		watch := exec.CommandContext(ctx, exe, "-i", "src/index.css", "-o", "src/index.gen.css", "--watch")
		watch.Dir = webDir
		watch.Stdout, watch.Stderr = os.Stdout, os.Stderr
		if err := watch.Start(); err != nil {
			fatal(fmt.Errorf("启动 Tailwind --watch 失败: %w", err))
		}
		defer func() {
			if watch.Process != nil {
				_ = watch.Process.Kill()
			}
		}()
		fmt.Println("👀 Tailwind 监听 web/src；Go 进程由 wf dev 管理")
		code := cli.Run([]string{"dev"})
		stop()
		if code != 0 {
			os.Exit(code)
		}
	default:
		fmt.Fprintf(os.Stderr, "未知命令 %q（可用：build / dev / css）\n", cmd)
		os.Exit(2)
	}
}

func must(err error) {
	if err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "✗", err)
	os.Exit(1)
}

func chdir(dir string) error { return os.Chdir(dir) }

// installDeps 按 package-lock.json 把前端依赖物化到 web/wf_modules（增量，零 Node）。
func installDeps() error {
	if code := cli.Run([]string{"install", "."}); code != 0 {
		return fmt.Errorf("wf install 失败（退出码 %d）", code)
	}
	return nil
}

func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("找不到 go.mod（请在仓库内运行）")
		}
		dir = parent
	}
}

// ---------------------------------------------------------------- Tailwind

// tailwindVersion 取 wf install 实际装进 store 的版本；没有 store 时回退 package.json。
func tailwindVersion(webDir string) (string, error) {
	var manifest struct {
		Root map[string]string `json:"root"`
	}
	data, err := os.ReadFile(filepath.Join(webDir, "wf_modules", ".store", "manifest.json"))
	if err == nil && json.Unmarshal(data, &manifest) == nil {
		if v := manifest.Root["tailwindcss"]; v != "" {
			return v, nil
		}
	}
	var pkg struct {
		DevDependencies map[string]string `json:"devDependencies"`
		Dependencies    map[string]string `json:"dependencies"`
	}
	data, err = os.ReadFile(filepath.Join(webDir, "package.json"))
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return "", err
	}
	v := pkg.DevDependencies["tailwindcss"]
	if v == "" {
		v = pkg.Dependencies["tailwindcss"]
	}
	v = strings.TrimLeft(v, "^~>=v")
	if v == "" {
		return "", errors.New("package.json 里没有 tailwindcss 版本")
	}
	return v, nil
}

func tailwindAsset() (string, error) {
	arch := map[string]string{"amd64": "x64", "arm64": "arm64"}[runtime.GOARCH]
	if arch == "" {
		return "", fmt.Errorf("Tailwind standalone 不支持 %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	switch runtime.GOOS {
	case "windows":
		return "tailwindcss-windows-" + arch + ".exe", nil
	case "darwin":
		return "tailwindcss-macos-" + arch, nil
	case "linux":
		if isMusl() {
			return "tailwindcss-linux-" + arch + "-musl", nil
		}
		return "tailwindcss-linux-" + arch, nil
	}
	return "", fmt.Errorf("Tailwind standalone 不支持 %s/%s", runtime.GOOS, runtime.GOARCH)
}

func isMusl() bool {
	if _, err := os.Stat("/etc/alpine-release"); err == nil {
		return true
	}
	matches, _ := filepath.Glob("/lib/ld-musl-*.so.1")
	return len(matches) > 0
}

func mustTailwind(webDir string) string {
	exe, err := ensureTailwind(webDir)
	if err != nil {
		fatal(err)
	}
	return exe
}

func ensureTailwind(webDir string) (string, error) {
	version, err := tailwindVersion(webDir)
	if err != nil {
		return "", err
	}
	asset, err := tailwindAsset()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(webDir, ".tools")
	name := "tailwindcss-" + version
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	exe := filepath.Join(dir, name)
	if info, err := os.Stat(exe); err == nil && info.Size() > 0 {
		return exe, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	url := fmt.Sprintf("https://github.com/tailwindlabs/tailwindcss/releases/download/v%s/%s", version, asset)
	fmt.Printf("⬇️  下载 Tailwind v%s standalone: %s\n", version, url)
	tmp := exe + ".download"
	if err := download(url, tmp); err != nil {
		return "", fmt.Errorf("下载 Tailwind 失败（可手动放到 %s）: %w", exe, err)
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, exe); err != nil {
		return "", err
	}
	return exe, nil
}

func download(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", url, resp.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func buildCSS(webDir, exe string, minify bool) error {
	args := []string{"-i", "src/index.css", "-o", "src/index.gen.css"}
	if minify {
		args = append(args, "--minify")
	}
	cmd := exec.Command(exe, args...)
	cmd.Dir = webDir
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Tailwind 编译失败: %w", err)
	}
	return nil
}

// ------------------------------------------------------------ 供应商图标

// genIcons 扫描 src/assets/providers，生成显式清单（webfront 不支持 import.meta.glob）。
func genIcons(webDir string) error {
	dir := filepath.Join(webDir, "src", "assets", "providers")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext == ".svg" || ext == ".png" {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	var b strings.Builder
	b.WriteString("// Code generated by `go run ./tools/web`; DO NOT EDIT.\n\n")
	b.WriteString("// 供应商图标文件名（图标本身由构建步骤拷到 dist/assets/providers/）。\n")
	b.WriteString("export const PROVIDER_ICONS: Record<string, string> = {\n")
	for _, name := range files {
		key := strings.TrimSuffix(name, filepath.Ext(name))
		fmt.Fprintf(&b, "  %q: %q,\n", key, name)
	}
	b.WriteString("}\n")
	return os.WriteFile(filepath.Join(webDir, "src", "assets", "providers.gen.ts"), []byte(b.String()), 0o644)
}

func copyIcons(webDir, dist string) error {
	src := filepath.Join(webDir, "src", "assets", "providers")
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	dst := filepath.Join(dist, "assets", "providers")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".svg" && ext != ".png" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// ------------------------------------------------------------- 外壳 HTML

// patchIndex 给 wf build 生成的 SPA 外壳补上项目需要的 head/属性。
func patchIndex(dist string) error {
	file := filepath.Join(dist, "index.html")
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	html := string(data)
	const head = `<meta name="viewport" content="width=device-width, initial-scale=1.0">` +
		`<meta name="color-scheme" content="dark">` +
		`<meta name="theme-color" content="#0d0d0f">` +
		`<link rel="icon" href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'%3E%3Crect width='32' height='32' rx='8' fill='%235e68d8'/%3E%3Ctext x='16' y='22' font-family='Arial' font-weight='700' font-size='16' fill='white' text-anchor='middle'%3EW%3C/text%3E%3C/svg%3E">`
	repl := []struct{ old, new string }{
		{`<html>`, `<html lang="zh-CN" class="dark">`},
		{`<meta charset="utf-8">`, `<meta charset="utf-8">` + head},
		{`<title>webfront</title>`, `<title>web2api</title>`},
		{`<div id="root"></div>`, `<div id="root"></div><noscript>web2api 控制台需要启用 JavaScript。</noscript>`},
	}
	for _, r := range repl {
		if !strings.Contains(html, r.old) {
			return fmt.Errorf("生成的 index.html 缺少 %q，webfront 的输出格式可能变了", r.old)
		}
		html = strings.Replace(html, r.old, r.new, 1)
	}
	return os.WriteFile(file, []byte(html), 0o644)
}
