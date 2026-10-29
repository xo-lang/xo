//go:build !windows

package rt

import (
	"syscall"
	"testing"
	"time"
)

// TestProcNotifyHangup subscribes to Hangup and User1, sends them to
// this process, and receives them (decision 0407).
func TestProcNotifyHangup(t *testing.T) {
	c := procNotify(nil, ListOf(Signal{Tag: 1}, Signal{Tag: 5}))
	defer procStopNotify(nil, c)
	for _, s := range []syscall.Signal{syscall.SIGHUP, syscall.SIGUSR1} {
		if err := syscall.Kill(syscall.Getpid(), s); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(2 * time.Second)
		for {
			if v := Chan_try_recv(nil, c); v.Some {
				if want := map[syscall.Signal]int{syscall.SIGHUP: 1, syscall.SIGUSR1: 5}[s]; v.V.Tag != want {
					t.Fatalf("%v arrived as tag %d, want %d", s, v.V.Tag, want)
				}
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("%v not delivered", s)
			}
			time.Sleep(time.Millisecond)
		}
	}
}
