package xotool

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

// Loopback test connections and the ephemeral ports. A connection whose
// side closes first stays in TIME_WAIT for 2 MSL (30 s on macOS, 60 s on
// Linux), holding its local port; the HTTP diff tests open thousands, so
// two overlapping internal/llvm runs once held every one of macOS's
// 16,384 ephemeral ports (16,807 in TIME_WAIT, 2026-10-16) and every new
// connection on the machine failed with "can't assign requested
// address". SO_LINGER 0 does not help a client that half closes first:
// its socket is in TIME_WAIT before it closes, and closing it then sends
// no reset.

// Private ports for test clients: below both the macOS (49152-65535) and
// the Linux (32768-60999) ephemeral ranges, so their TIME_WAIT never
// takes a port from anything else.
const (
	privFirst = 20000
	privLast  = 32767
)

// privNext is the next private port; it starts at an offset of the
// process so two test processes rarely try the same ones.
var privNext atomic.Int64

func init() {
	privNext.Store(int64(os.Getpid() % (privLast - privFirst + 1)))
}

// DialLoopback dials addr (a loopback TCP address) from a private local
// port with SO_REUSEADDR: a port in TIME_WAIT toward another server is
// reused, and one toward this server is skipped. A lap of the range
// serves 12,768 connections to one server per 2 MSL. Where SO_REUSEADDR
// cannot be set (Windows) it dials as net.DialTimeout does.
func DialLoopback(addr string, timeout time.Duration) (net.Conn, error) {
	if reuseAddr == nil {
		c, err := net.DialTimeout("tcp", addr, timeout)
		return c, DialErr(err)
	}
	n := privLast - privFirst + 1
	var err error
	for try := 0; try < n; try++ {
		p := privFirst + int(privNext.Add(1)%int64(n))
		d := net.Dialer{Timeout: timeout, LocalAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: p}, Control: reuseAddr}
		var c net.Conn
		c, err = d.Dial("tcp", addr)
		if err == nil {
			return c, nil
		}
		// the port is bound by another socket, or in TIME_WAIT toward
		// this server (macOS EADDRINUSE, Linux EADDRNOTAVAIL)
		if !errors.Is(err, syscall.EADDRINUSE) && !errors.Is(err, syscall.EADDRNOTAVAIL) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("%v (every private port %d-%d in use toward %s)", err, privFirst, privLast, addr)
}

// DialErr adds the count of ephemeral ports in TIME_WAIT to a dial error
// that ran out of them ("can't assign requested address"), so the test
// says what happened instead of looking like a server's fault.
func DialErr(err error) error {
	if err == nil || !errors.Is(err, syscall.EADDRNOTAVAIL) {
		return err
	}
	if n, size, e := TimeWait(); e == nil {
		return fmt.Errorf("%w (ephemeral ports exhausted: %d of %d in TIME_WAIT; another test run's connections?)", err, n, size)
	}
	return err
}

// PortsLow returns a message when more than three quarters of the
// ephemeral ports are in TIME_WAIT, else "": a test that opens many
// connections fails with it at once instead of with a misleading error
// later.
func PortsLow() string {
	n, size, err := TimeWait()
	if err != nil || n*4 < size*3 {
		return ""
	}
	return fmt.Sprintf("ephemeral ports exhausted: %d of %d in TIME_WAIT (wait 2 MSL, or for the other test run to end)", n, size)
}

// NeedPorts fails tb at once with PortsLow's message when the ephemeral
// ports run out (not under -short: the tests that open many connections
// skip there).
func NeedPorts(tb interface {
	Helper()
	Fatal(...any)
}) {
	// the test binary's -short flag (xotool is not only test code, so
	// not package testing)
	if f := flag.Lookup("test.short"); f != nil && f.Value.String() == "true" {
		return
	}
	tb.Helper()
	if m := PortsLow(); m != "" {
		tb.Fatal(m)
	}
}

// TimeWait counts this host's TCP connections in TIME_WAIT whose local
// port is ephemeral, and the size of the ephemeral range (darwin:
// netstat and sysctl; linux: /proc).
func TimeWait() (n, size int, err error) {
	switch runtime.GOOS {
	case "darwin":
		first, last, err := sysctlRange()
		if err != nil {
			return 0, 0, err
		}
		out, err := exec.Command("netstat", "-an", "-p", "tcp").Output()
		if err != nil {
			return 0, 0, err
		}
		for _, l := range strings.Split(string(out), "\n") {
			f := strings.Fields(l)
			if len(f) < 6 || f[5] != "TIME_WAIT" {
				continue
			}
			p, _ := strconv.Atoi(f[3][strings.LastIndexByte(f[3], '.')+1:])
			if p >= first && p <= last {
				n++
			}
		}
		return n, last - first + 1, nil
	case "linux":
		b, err := os.ReadFile("/proc/sys/net/ipv4/ip_local_port_range")
		if err != nil {
			return 0, 0, err
		}
		var first, last int
		if _, err := fmt.Sscan(string(b), &first, &last); err != nil {
			return 0, 0, err
		}
		for _, name := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
			f, err := os.Open(name)
			if err != nil {
				continue
			}
			s := bufio.NewScanner(f)
			for s.Scan() {
				// sl local_address rem_address st ...: st 06 is TIME_WAIT
				x := strings.Fields(s.Text())
				if len(x) < 4 || x[3] != "06" {
					continue
				}
				p, _ := strconv.ParseInt(x[1][strings.LastIndexByte(x[1], ':')+1:], 16, 32)
				if int(p) >= first && int(p) <= last {
					n++
				}
			}
			f.Close()
		}
		return n, last - first + 1, nil
	}
	return 0, 0, fmt.Errorf("TIME_WAIT count: not on %s", runtime.GOOS)
}

func sysctlRange() (int, int, error) {
	out, err := exec.Command("sysctl", "-n", "net.inet.ip.portrange.first", "net.inet.ip.portrange.last").Output()
	if err != nil {
		return 0, 0, err
	}
	var first, last int
	if _, err := fmt.Sscan(string(out), &first, &last); err != nil {
		return 0, 0, err
	}
	return first, last, nil
}
