package gateway

import (
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	// sseHeartbeatInterval 心跳检查间隔。必须明显小于 sseIdleThreshold，
	// 否则第一次检查要等到 interval 之后，静默期早于检查点就发不出心跳。
	sseHeartbeatInterval = 5 * time.Second
	// sseIdleThreshold 空闲超过该时长才发心跳，避免打断正常输出节奏。
	sseIdleThreshold = 10 * time.Second
)

// sseStream 串行化 SSE 写入，并在空闲期发送注释行心跳。
//
// 心跳解决两件事：模型长时间不产出（思考、排队）时，Codex 的
// stream_idle_timeout_ms（默认 5 分钟）和反向代理的 idle timeout 都会把连接
// 判死；注释行（以 ':' 开头）不是事件，SSE 客户端会直接跳过。
type sseStream struct {
	w  http.ResponseWriter
	rc *http.ResponseController

	mu   sync.Mutex
	last time.Time
	err  error
	done chan struct{}
	once sync.Once
	wg   sync.WaitGroup
}

func newSSEStream(w http.ResponseWriter) *sseStream {
	return &sseStream{w: w, rc: http.NewResponseController(w), done: make(chan struct{})}
}

// Start 写出 SSE 响应头并启动心跳协程。此后所有写入都应走 Write。
func (s *sseStream) Start() error {
	h := s.w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	// 关掉 nginx 一类反向代理的响应缓冲，否则分片会被攒着一起发，
	// 客户端看到的是「卡住很久 → 一次性吐完」，长回答还会被缓冲层截断。
	h.Set("X-Accel-Buffering", "no")
	s.w.WriteHeader(http.StatusOK)

	s.mu.Lock()
	s.last = time.Now()
	s.mu.Unlock()

	if err := s.rc.Flush(); err != nil {
		return err
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.heartbeat()
	}()
	return nil
}

// heartbeat 在空闲期写 ": keepalive"，保持连接活跃。
func (s *sseStream) heartbeat() {
	t := time.NewTicker(sseHeartbeatInterval)
	defer t.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-t.C:
			s.mu.Lock()
			if time.Since(s.last) >= sseIdleThreshold {
				if _, err := io.WriteString(s.w, ": keepalive\n\n"); err != nil {
					s.err = err
				} else if err := s.rc.Flush(); err != nil {
					s.err = err
				} else {
					s.last = time.Now()
				}
			}
			s.mu.Unlock()
		}
	}
}

// Write 写一条 SSE 事件：可选的 event 名 + 一行 data，空行分隔后 flush。
// event 为空时只写 data 行（Chat Completions 的惯例）。
func (s *sseStream) Write(event string, payload []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	if event != "" {
		if _, err := io.WriteString(s.w, "event: "+event+"\n"); err != nil {
			s.err = err
			return err
		}
	}
	if _, err := io.WriteString(s.w, "data: "); err != nil {
		s.err = err
		return err
	}
	if _, err := s.w.Write(payload); err != nil {
		s.err = err
		return err
	}
	if _, err := io.WriteString(s.w, "\n\n"); err != nil {
		s.err = err
		return err
	}
	if err := s.rc.Flush(); err != nil {
		s.err = err
		return err
	}
	s.last = time.Now()
	return nil
}

// Err 返回写入过程中出现过的第一个错误（客户端断开等）。
func (s *sseStream) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Close 停止心跳协程并等待它退出，保证返回后不再有写入。可重复调用。
func (s *sseStream) Close() {
	s.once.Do(func() {
		close(s.done)
		s.wg.Wait()
	})
}
