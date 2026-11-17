package logging

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	l := NewTest(&buf)
	l.SetLevel(LevelWarn)
	l.Debugf("debug-line")
	l.Infof("info-line")
	l.Warnf("warn-line")
	l.Errorf("error-line")
	out := buf.String()
	if strings.Contains(out, "debug-line") || strings.Contains(out, "info-line") {
		t.Fatalf("低于 warn 的日志不应输出: %s", out)
	}
	if !strings.Contains(out, "warn-line") || !strings.Contains(out, "error-line") {
		t.Fatalf("warn/error 应输出: %s", out)
	}
	if !strings.Contains(out, "[WARN ]") || !strings.Contains(out, "[ERROR]") {
		t.Fatalf("输出应带级别标记: %s", out)
	}
}

func TestPrintfIsInfo(t *testing.T) {
	var buf bytes.Buffer
	l := NewTest(&buf)
	l.Printf("legacy %s", "call")
	if !strings.Contains(buf.String(), "[INFO ]") || !strings.Contains(buf.String(), "legacy call") {
		t.Fatalf("Printf 应按 info 记录: %s", buf.String())
	}
}

func TestFileWriterRotatesAndPrunes(t *testing.T) {
	dir := t.TempDir()
	// 写入一个过期文件，确认启动时会被清理。
	old := filepath.Join(dir, logFilePrefix+"20200101.log")
	if err := os.WriteFile(old, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	w, err := newFileWriter(dir, 7, 1) // 1MB 上限
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if _, err := w.Write([]byte("hello\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("超过保留天数的日志文件应被删除: %v", err)
	}
	files := LogFiles(dir)
	if len(files) != 1 {
		t.Fatalf("应只保留当天日志文件，got %v", files)
	}
	if !strings.HasPrefix(files[0], logFilePrefix) {
		t.Fatalf("文件名格式不对: %s", files[0])
	}
	// 达到大小上限后应切分到 -2 文件。
	if _, err := w.Write(bytes.Repeat([]byte("x"), 1<<20)); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("next")); err != nil {
		t.Fatal(err)
	}
	files = LogFiles(dir)
	if len(files) != 2 {
		t.Fatalf("超过大小上限应切分，got %v", files)
	}
}

func TestDirDashDisablesFileLogging(t *testing.T) {
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	l := New(Options{Level: "info", Dir: "-", Console: false})
	defer l.Close()
	if l.file != nil {
		t.Fatal(`Dir="-" 不应启用文件日志`)
	}
	if _, err := os.Stat(filepath.Join(dir, "-")); !os.IsNotExist(err) {
		t.Fatalf(`Dir="-" 不应创建名为 "-" 的目录: %v`, err)
	}
}

func TestSlogBridge(t *testing.T) {
	var buf bytes.Buffer
	l := NewTest(&buf)
	logger := slog.New(SlogHandler(l))
	logger.Warn("模型测试失败", "model", "gpt-4o", "error", "upstream 500")
	out := buf.String()
	for _, want := range []string{"[WARN ]", "模型测试失败", "model=gpt-4o", "error=upstream 500"} {
		if !strings.Contains(out, want) {
			t.Fatalf("slog 输出缺少 %q: %s", want, out)
		}
	}
}
