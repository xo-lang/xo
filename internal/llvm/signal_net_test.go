package llvm

import (
	"net"
	"strconv"
	"testing"
	"time"
)

// TestNativeSignalNetNoTasks: a program whose only waits are std/udp's and
// std/dns's (testdata/signal_net, no task anywhere in its code) keeps the
// two phase shutdown (core 7.5): the first signal cancels nothing, the
// second unwinds the blocked wait at once and main's defer runs, exit 0.
// Before 2026-11-03 the compiler built such a program without the
// scheduler (std/dns and std/udp were not counted as concurrency): SIGTERM
// killed it with no defer run and the udp mode did not build.
func TestNativeSignalNetNoTasks(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"udp", "dns"} {
		for _, bk := range nativeBackends() {
			t.Run(mode+"/"+bk, func(t *testing.T) {
				t.Parallel()
				env := []string{"MODE=" + mode}
				if mode == "dns" {
					// a DNS server that reads and never answers
					pc, err := net.ListenPacket("udp", "127.0.0.1:0")
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { pc.Close() })
					go func() {
						buf := make([]byte, 512)
						for {
							if _, _, err := pc.ReadFrom(buf); err != nil {
								return
							}
						}
					}()
					env = append(env, "PORT="+strconv.Itoa(pc.LocalAddr().(*net.UDPAddr).Port))
				}
				c := startNativeIn(t, bk, "signal_net", env...)
				c.expect("ready")
				time.Sleep(200 * time.Millisecond)
				c.signal()
				time.Sleep(300 * time.Millisecond)
				select {
				case line := <-c.lines:
					t.Fatalf("output after the first signal: %q", line)
				default:
				}
				c.signal()
				start := time.Now()
				code, rest := c.wait()
				if code != 0 {
					t.Fatalf("exit %d (output %q)", code, rest)
				}
				if !(rest == "read ended\ncleanup ran\n" || rest == "lookup ended\ncleanup ran\n" || rest == "cleanup ran\n") {
					t.Errorf("output after the second signal %q", rest)
				}
				// the wait unwinds at once, not at the lookup's own timeout
				if d := time.Since(start); d > 1500*time.Millisecond {
					t.Errorf("exit %v after the second signal", d)
				}
			})
		}
	}
}
