// Package testredis is a small RESP test fixture for session integration tests.
package testredis

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type entry struct {
	value   string
	expires time.Time
}
type Server struct {
	listener net.Listener
	mu       sync.Mutex
	values   map[string]entry
	Password string
	Database string
}

func Start(t *testing.T) *Server {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{listener: l, values: make(map[string]entry)}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go s.serve(c)
		}
	}()
	return s
}

func (s *Server) URL() string { return "redis://" + s.listener.Addr().String() + "/0" }
func (s *Server) Close()      { _ = s.listener.Close() }

func (s *Server) serve(c net.Conn) {
	defer c.Close()
	b := bufio.NewReader(c)
	authorized := s.Password == ""
	selected := s.Database == "" || s.Database == "0"
	for {
		line, err := b.ReadString('\n')
		if err != nil {
			return
		}
		n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "*")))
		if err != nil {
			return
		}
		args := make([]string, n)
		for i := range args {
			line, err = b.ReadString('\n')
			if err != nil {
				return
			}
			size, e := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "$")))
			if e != nil || size < 0 || size > 1<<20 {
				return
			}
			data := make([]byte, size+2)
			if _, err = io.ReadFull(b, data); err != nil {
				return
			}
			args[i] = string(data[:size])
		}
		if len(args) == 0 {
			return
		}
		if args[0] == "AUTH" {
			authorized = args[len(args)-1] == s.Password
			if authorized {
				fmt.Fprint(c, "+OK\r\n")
			} else {
				fmt.Fprint(c, "-ERR password\r\n")
			}
			continue
		}
		if !authorized {
			fmt.Fprint(c, "-NOAUTH\r\n")
			continue
		}
		if args[0] == "SELECT" {
			selected = args[1] == s.Database
			if selected {
				fmt.Fprint(c, "+OK\r\n")
			} else {
				fmt.Fprint(c, "-ERR database\r\n")
			}
			continue
		}
		if !selected {
			fmt.Fprint(c, "-ERR select required\r\n")
			continue
		}
		s.mu.Lock()
		for k, v := range s.values {
			if !v.expires.IsZero() && !time.Now().Before(v.expires) {
				delete(s.values, k)
			}
		}
		switch args[0] {
		case "SET":
			v := entry{value: args[2]}
			if len(args) == 5 && args[3] == "EX" {
				ttl, _ := strconv.Atoi(args[4])
				v.expires = time.Now().Add(time.Duration(ttl) * time.Second)
			}
			s.values[args[1]] = v
			fmt.Fprint(c, "+OK\r\n")
		case "GET":
			v, ok := s.values[args[1]]
			if !ok {
				fmt.Fprint(c, "$-1\r\n")
			} else {
				fmt.Fprintf(c, "$%d\r\n%s\r\n", len(v.value), v.value)
			}
		case "DEL":
			_, ok := s.values[args[1]]
			delete(s.values, args[1])
			if ok {
				fmt.Fprint(c, ":1\r\n")
			} else {
				fmt.Fprint(c, ":0\r\n")
			}
		case "EVAL":
			v := s.values[args[3]]
			n, _ := strconv.Atoi(v.value)
			n++
			if n == 1 {
				v.expires = time.Now().Add(15 * time.Minute)
			}
			v.value = strconv.Itoa(n)
			s.values[args[3]] = v
			fmt.Fprintf(c, ":%d\r\n", n)
		default:
			fmt.Fprint(c, "-ERR unsupported\r\n")
		}
		s.mu.Unlock()
	}
}
