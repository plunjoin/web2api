// Package logging 提供 web2api 的分级运行日志：
//
//   - 级别 debug / info / warn / error，由 server.log_level 控制；
//   - 同时写 stdout（便于 systemd / Docker 采集）与 data/logs 下的日志文件；
//   - 文件按天命名，超过大小上限自动切分，并清理超过保留天数的旧文件；
//   - 兼容既有的 *log.Logger.Printf 调用，并把标准库 slog 统一接到这里。
//
// 日志文件的写入失败不能影响业务：文件不可写时退回只写 stdout。
package logging

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Level 日志级别。数值越小越详细。
type Level int32

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// ParseLevel 解析配置里的级别名，无法识别时按 info 处理。
func ParseLevel(raw string) Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug", "trace", "verbose":
		return LevelDebug
	case "warn", "warning":
		return LevelWarn
	case "error", "fatal":
		return LevelError
	default:
		return LevelInfo
	}
}

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "INFO"
	}
}

// Options 日志系统参数。零值可用：info、仅 stdout。
type Options struct {
	Level     string // debug | info | warn | error
	Dir       string // 日志目录；空 = 不写文件
	MaxDays   int    // 文件保留天数；<=0 用 7
	MaxSizeMB int    // 单文件大小上限；<=0 用 50
	Console   bool   // 是否写 stdout
}

// Logger 并发安全的分级日志器。
type Logger struct {
	level   atomic.Int32
	prefix  string
	console io.Writer
	file    *fileWriter // nil = 不写文件
	mu      sync.Mutex  // 串行化一次完整日志行的写入
}

const defaultPrefix = "[web2api] "

// New 按配置创建日志器。目录创建或文件打开失败时不返回错误，而是降级为
// 仅 stdout（错误会通过标准库打印一次）。
func New(opts Options) *Logger {
	l := &Logger{
		prefix:  defaultPrefix,
		console: os.Stdout,
	}
	l.level.Store(int32(ParseLevel(opts.Level)))
	if opts.Dir != "" {
		if w, err := newFileWriter(opts.Dir, opts.MaxDays, opts.MaxSizeMB); err != nil {
			fmt.Fprintf(os.Stderr, "[web2api] 日志文件不可用，仅输出到 stdout: %v\n", err)
		} else {
			l.file = w
		}
	}
	if !opts.Console {
		l.console = nil
	}
	return l
}

// NewTest 创建一个只写 w、级别为 debug 的日志器（测试用）。
func NewTest(w io.Writer) *Logger {
	l := &Logger{prefix: defaultPrefix, console: w}
	l.level.Store(int32(LevelDebug))
	return l
}

// SetLevel 调整级别（-v 等运行时覆盖）。
func (l *Logger) SetLevel(level Level) { l.level.Store(int32(level)) }

// Level 当前级别。
func (l *Logger) Level() Level { return Level(l.level.Load()) }

// Close 关闭日志文件。
func (l *Logger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	return l.file.Close()
}

// Printf 兼容标准库 *log.Logger 的调用方式，按 info 记录。
func (l *Logger) Printf(format string, args ...any) { l.logf(LevelInfo, format, args...) }

// Println 兼容标准库调用方式，按 info 记录。
func (l *Logger) Println(args ...any) {
	if l == nil {
		return
	}
	l.logf(LevelInfo, "%s", fmt.Sprint(args...))
}

// Debugf 记录 debug 日志。
func (l *Logger) Debugf(format string, args ...any) { l.logf(LevelDebug, format, args...) }

// Infof 记录 info 日志。
func (l *Logger) Infof(format string, args ...any) { l.logf(LevelInfo, format, args...) }

// Warnf 记录 warn 日志。
func (l *Logger) Warnf(format string, args ...any) { l.logf(LevelWarn, format, args...) }

// Errorf 记录 error 日志。
func (l *Logger) Errorf(format string, args ...any) { l.logf(LevelError, format, args...) }

// Fatalf 记录 error 日志后退出进程（兼容标准库 log.Fatalf 的使用场景）。
func (l *Logger) Fatalf(format string, args ...any) {
	l.logf(LevelError, format, args...)
	if l.file != nil {
		_ = l.file.Close()
	}
	os.Exit(1)
}

// Enabled 该级别是否会输出。
func (l *Logger) Enabled(level Level) bool {
	return l != nil && level >= Level(l.level.Load())
}

func (l *Logger) logf(level Level, format string, args ...any) {
	if l == nil || !l.Enabled(level) {
		return
	}
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("%s [%-5s] %s%s\n", time.Now().Format("2006-01-02 15:04:05.000"), level, l.prefix, msg)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.console != nil {
		_, _ = io.WriteString(l.console, line)
	}
	if l.file != nil {
		_, _ = l.file.Write([]byte(line))
	}
}

// ==================== slog 桥接 ====================

// SlogHandler 把标准库 slog 记录转发到 Logger，使内嵌引擎里的
// slog.Info/Warn/Error 与管理台、网关使用同一套级别和文件。
func SlogHandler(l *Logger) slog.Handler { return &slogBridge{logger: l} }

type slogBridge struct {
	logger *Logger
	attrs  []slog.Attr
	group  string
}

func (h *slogBridge) Enabled(_ context.Context, level slog.Level) bool {
	return h.logger.Enabled(slogLevel(level))
}

func (h *slogBridge) Handle(_ context.Context, r slog.Record) error {
	var sb strings.Builder
	sb.WriteString(r.Message)
	writeAttr := func(a slog.Attr) {
		if a.Key == "" {
			return
		}
		key := a.Key
		if h.group != "" {
			key = h.group + "." + key
		}
		sb.WriteString(" " + key + "=" + attrText(a.Value))
	}
	for _, a := range h.attrs {
		writeAttr(a)
	}
	r.Attrs(func(a slog.Attr) bool {
		writeAttr(a)
		return true
	})
	h.logger.logf(slogLevel(r.Level), "%s", sb.String())
	return nil
}

func (h *slogBridge) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := *h
	next.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &next
}

func (h *slogBridge) WithGroup(name string) slog.Handler {
	next := *h
	if name != "" {
		if h.group != "" {
			next.group = h.group + "." + name
		} else {
			next.group = name
		}
	}
	return &next
}

func attrText(v slog.Value) string {
	v = v.Resolve()
	switch v.Kind() {
	case slog.KindString:
		return v.String()
	case slog.KindTime:
		return v.Time().Format(time.RFC3339)
	default:
		return fmt.Sprint(v.Any())
	}
}

func slogLevel(level slog.Level) Level {
	switch {
	case level >= slog.LevelError:
		return LevelError
	case level >= slog.LevelWarn:
		return LevelWarn
	case level >= slog.LevelInfo:
		return LevelInfo
	default:
		return LevelDebug
	}
}

// ==================== 默认 logger ====================

var (
	stdMu sync.RWMutex
	std   = &Logger{prefix: defaultPrefix, console: os.Stdout}
)

// SetDefault 设置包级默认 logger（main 启动时调用）。
func SetDefault(l *Logger) {
	if l == nil {
		return
	}
	stdMu.Lock()
	std = l
	stdMu.Unlock()
	// 让标准库 log 的错误输出也走统一日志，而不再直接写 stderr。
	log.SetOutput(levelWriter{l})
	log.SetFlags(0)
	log.SetPrefix("")
}

// Default 返回包级默认 logger。
func Default() *Logger {
	stdMu.RLock()
	defer stdMu.RUnlock()
	return std
}

// Debugf 用默认 logger 记录 debug 日志。
func Debugf(format string, args ...any) { Default().Debugf(format, args...) }

// Infof 用默认 logger 记录 info 日志。
func Infof(format string, args ...any) { Default().Infof(format, args...) }

// Warnf 用默认 logger 记录 warn 日志。
func Warnf(format string, args ...any) { Default().Warnf(format, args...) }

// Errorf 用默认 logger 记录 error 日志。
func Errorf(format string, args ...any) { Default().Errorf(format, args...) }

// levelWriter 把标准库 log 的输出（级别未知）按 info 写入。
type levelWriter struct{ l *Logger }

func (w levelWriter) Write(p []byte) (int, error) {
	msg := strings.TrimRight(string(p), "\r\n")
	w.l.Infof("%s", msg)
	return len(p), nil
}

// ==================== 文件轮转 ====================

// fileWriter 按天 + 大小切分日志文件：
//
//	web2api-20261117.log     当天第 1 个文件
//	web2api-20261117-2.log   当天第 2 个文件（超过大小上限后）
type fileWriter struct {
	mu       sync.Mutex
	dir      string
	maxBytes int64
	maxDays  int
	day      string
	seq      int
	file     *os.File
	size     int64
}

const logFilePrefix = "web2api-"

func newFileWriter(dir string, maxDays, maxSizeMB int) (*fileWriter, error) {
	if maxDays <= 0 {
		maxDays = 7
	}
	if maxSizeMB <= 0 {
		maxSizeMB = 50
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	w := &fileWriter{dir: dir, maxBytes: int64(maxSizeMB) << 20, maxDays: maxDays}
	if err := w.open(time.Now()); err != nil {
		return nil, err
	}
	w.prune(time.Now())
	return w, nil
}

func (w *fileWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now()
	if w.file == nil || w.day != now.Format("20060102") || (w.size > 0 && w.size+int64(len(p)) > w.maxBytes) {
		if err := w.rotate(now); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *fileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

// rotate 在当前序号基础上打开可写文件；当天第一个文件已写满时递增序号。
func (w *fileWriter) rotate(now time.Time) error {
	day := now.Format("20060102")
	if w.day != day {
		w.day = day
		w.seq = 1
	}
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	// 同一天多个文件时跳过已写满的历史文件（进程重启后也能接着写）。
	for {
		name := w.fileName(day, w.seq)
		f, err := os.OpenFile(filepath.Join(w.dir, name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		info, err := f.Stat()
		if err != nil {
			_ = f.Close()
			return err
		}
		if info.Size() >= w.maxBytes && w.seq < 10000 {
			_ = f.Close()
			w.seq++
			continue
		}
		w.file = f
		w.size = info.Size()
		return nil
	}
}

func (w *fileWriter) fileName(day string, seq int) string {
	if seq <= 1 {
		return logFilePrefix + day + ".log"
	}
	return fmt.Sprintf("%s%s-%d.log", logFilePrefix, day, seq)
}

// open 首次打开当天文件。
func (w *fileWriter) open(now time.Time) error {
	w.day = ""
	return w.rotate(now)
}

// prune 删除超过保留天数的日志文件（按文件名里的日期判断）。
func (w *fileWriter) prune(now time.Time) {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return
	}
	cutoff := now.AddDate(0, 0, -(w.maxDays - 1)).Format("20060102")
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, logFilePrefix) || !strings.HasSuffix(name, ".log") {
			continue
		}
		day := strings.TrimSuffix(strings.TrimPrefix(name, logFilePrefix), ".log")
		day = strings.SplitN(day, "-", 2)[0]
		if len(day) != 8 || day >= cutoff {
			continue
		}
		_ = os.Remove(filepath.Join(w.dir, name))
	}
}

// LogFiles 返回目录下按时间倒序排列的日志文件名（供运维/测试检查）。
func LogFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), logFilePrefix) && strings.HasSuffix(entry.Name(), ".log") {
			names = append(names, entry.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	return names
}
