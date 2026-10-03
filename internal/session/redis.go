// Package session implements the Redis commands used for administrator sessions.
package session

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Redis struct {
	address, username, password, database string
	secure                                bool
}

// New accepts redis://[user:password@]host:port/db and rediss:// for TLS.
func New(raw string) (*Redis, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "redis" && u.Scheme != "rediss") || u.Hostname() == "" || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("请填写有效的 redis:// 或 rediss:// 连接地址")
	}
	db := strings.TrimPrefix(u.Path, "/")
	if db == "" {
		db = "0"
	}
	n, err := strconv.Atoi(db)
	if err != nil || n < 0 {
		return nil, errors.New("Redis 数据库编号无效")
	}
	port := u.Port()
	if port == "" {
		port = "6379"
	}
	r := &Redis{address: net.JoinHostPort(u.Hostname(), port), database: strconv.Itoa(n), secure: u.Scheme == "rediss"}
	if u.User != nil {
		r.username = u.User.Username()
		r.password, _ = u.User.Password()
	}
	return r, nil
}

// Command uses a bounded connection per operation; no background resources remain.
func (r *Redis) Command(ctx context.Context, args ...string) (string, error) {
	d := &net.Dialer{Timeout: 3 * time.Second}
	var c net.Conn
	var err error
	if r.secure {
		c, err = (&tls.Dialer{NetDialer: d, Config: &tls.Config{MinVersion: tls.VersionTLS12}}).DialContext(ctx, "tcp", r.address)
	} else {
		c, err = d.DialContext(ctx, "tcp", r.address)
	}
	if err != nil {
		return "", err
	}
	defer c.Close()
	deadline := time.Now().Add(3 * time.Second)
	if v, ok := ctx.Deadline(); ok && v.Before(deadline) {
		deadline = v
	}
	_ = c.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { _ = c.Close() })
	defer stop()
	b := bufio.NewReader(c)
	if r.password != "" || r.username != "" {
		auth := []string{"AUTH", r.password}
		if r.username != "" {
			auth = []string{"AUTH", r.username, r.password}
		}
		if _, err = command(c, b, auth); err != nil {
			return "", err
		}
	}
	if r.database != "0" {
		if _, err = command(c, b, []string{"SELECT", r.database}); err != nil {
			return "", err
		}
	}
	return command(c, b, args)
}

func command(w io.Writer, b *bufio.Reader, args []string) (string, error) {
	var out strings.Builder
	fmt.Fprintf(&out, "*%d\r\n", len(args))
	for _, v := range args {
		fmt.Fprintf(&out, "$%d\r\n%s\r\n", len(v), v)
	}
	if _, err := io.WriteString(w, out.String()); err != nil {
		return "", err
	}
	line, err := b.ReadString('\n')
	if err != nil {
		return "", err
	}
	if len(line) < 3 || !strings.HasSuffix(line, "\r\n") {
		return "", errors.New("invalid Redis response")
	}
	v := line[1 : len(line)-2]
	switch line[0] {
	case '+', ':':
		return v, nil
	case '-':
		return "", errors.New("Redis command failed") // Do not disclose server details or secrets.
	case '$':
		n, err := strconv.Atoi(v)
		if err != nil || n < -1 || n > 1<<20 {
			return "", errors.New("invalid Redis bulk response")
		}
		if n == -1 {
			return "", nil
		}
		data := make([]byte, n+2)
		if _, err = io.ReadFull(b, data); err != nil {
			return "", err
		}
		if string(data[n:]) != "\r\n" {
			return "", errors.New("invalid Redis terminator")
		}
		return string(data[:n]), nil
	}
	return "", errors.New("unsupported Redis response")
}
