package stdtest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestIdleConnections40k is the proof of decision 0449: an Xo HTTP server
// holds 40,000 idle HTTP/1.1 connections (bench/perf/idleconn), every one
// still answers a second request, and the server has fewer than 20,000
// mappings. Before 0449 each connection owned a fiber stack, two
// mappings (the stack and its guard page), so the default
// vm.max_map_count of 65,530 stopped a server near 32,000 connections.
//
// Linux only (it reads /proc), and heavy (it needs 80,000 descriptors and
// about a minute): it runs when XO_TEST_IDLE40K=1, on the offload host.
func TestIdleConnections40k(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("reads /proc/PID/maps")
	}
	if os.Getenv("XO_TEST_IDLE40K") != "1" {
		t.Skip("set XO_TEST_IDLE40K=1 (heavy: 40,000 connections, linux)")
	}
	raiseNoFile(t, 200000)
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := testtmp.Dir("xo-idleconn-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	srv := filepath.Join(dir, "srv")
	cli := filepath.Join(dir, "cli")
	idle := filepath.Join(root, "bench", "perf", "idleconn")
	if out, err := exec.Command(xo, "build", "-o", srv, filepath.Join(idle, "xo")).CombinedOutput(); err != nil {
		t.Fatalf("xo build: %v\n%s", err, out)
	}
	b := exec.Command("go", "build", "-o", cli, ".")
	b.Dir = filepath.Join(idle, "client")
	if out, err := b.CombinedOutput(); err != nil {
		t.Fatalf("go build client: %v\n%s", err, out)
	}

	const port = "18040"
	s := exec.Command(srv)
	s.Env = append(os.Environ(), "PORT="+port)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		s.Process.Kill()
		s.Wait()
	}()
	time.Sleep(2 * time.Second)

	c := exec.Command(cli, "-addr", "127.0.0.1:"+port, "-n", "40000",
		"-pid", fmt.Sprint(s.Process.Pid), "-settle", "10s", "-maxmaps", "20000", "-again", "-srcs", "3")
	out, err := c.CombinedOutput()
	t.Logf("%s", lastLines(string(out), 8))
	if err != nil {
		t.Fatalf("40,000 idle connections: %v", err)
	}
}

// raiseNoFile raises the soft descriptor limit to n for this process and
// its children.
func raiseNoFile(t *testing.T, n uint64) {
	t.Helper()
	var lim syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &lim); err != nil {
		t.Fatal(err)
	}
	if lim.Cur >= n {
		return
	}
	want := lim
	want.Cur = n
	if want.Max < n {
		want.Max = n
	}
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &want); err != nil {
		t.Skipf("cannot raise RLIMIT_NOFILE to %d (hard limit %d): %v", n, lim.Max, err)
	}
}

// lastLines is the last n lines of s.
func lastLines(s string, n int) string {
	ls := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(ls) > n {
		ls = ls[len(ls)-n:]
	}
	return strings.Join(ls, "\n")
}
