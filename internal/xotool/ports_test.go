package xotool

import (
	"io"
	"net"
	"runtime"
	"testing"
	"time"
)

// TestDialLoopback: the client closes first (its side holds TIME_WAIT),
// from a private port, and the next connection gets another one.
func TestDialLoopback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("plain dials without SO_REUSEADDR")
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() { io.ReadAll(c); c.Write([]byte("ok")); c.Close() }()
		}
	}()
	seen := map[int]bool{}
	for i := 0; i < 50; i++ {
		c, err := DialLoopback(l.Addr().String(), 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		p := c.LocalAddr().(*net.TCPAddr).Port
		if p < privFirst || p > privLast || seen[p] {
			t.Fatalf("connection %d: local port %d (want a fresh one in %d-%d)", i, p, privFirst, privLast)
		}
		seen[p] = true
		c.(*net.TCPConn).CloseWrite()
		if b, _ := io.ReadAll(c); string(b) != "ok" {
			t.Fatalf("read %q", b)
		}
		c.Close()
	}
	if _, _, err := TimeWait(); err != nil && runtime.GOOS != "windows" {
		t.Error(err)
	}
}
