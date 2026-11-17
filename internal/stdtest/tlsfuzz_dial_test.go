package stdtest

import (
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"
	"testing"
)

// TestTlfSendWaitsForAPort: a dial that finds no free local port
// (EADDRNOTAVAIL) is tried again, not reported as the server's answer
// (seed 3 of TestTlsFuzz lost 772 cases to "can't assign requested
// address" on the Mac, 2026-11-17).
func TestTlfSendWaitsForAPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Write([]byte{21, 3, 3, 0, 2, 2, 40})
			c.Close()
		}
	}()
	real := tlfDial
	defer func() { tlfDial = real }()
	fails := 3
	tlfDial = func(addr string) (net.Conn, error) {
		if fails > 0 {
			fails--
			return nil, &net.OpError{Op: "dial", Net: "tcp", Err: &os.SyscallError{Syscall: "connect", Err: syscall.EADDRNOTAVAIL}}
		}
		return real(addr)
	}
	got := tlfSend(ln.Addr().String(), []byte{22, 3, 1, 0, 0})
	if strings.HasPrefix(got, "dial ") || !strings.Contains(got, "alert 2 40") {
		t.Fatalf("got %q, want the server's alert 2 40", got)
	}
	if fails != 0 {
		t.Fatal(fmt.Sprint("dials left: ", fails))
	}
}
