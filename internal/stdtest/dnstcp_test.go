package stdtest

// std/dns's TCP fallback waits in the task, not the worker thread (task
// #115): the dns stress program (std/evidence/dns/stress, its UDP and TCP
// server in the same program as the lookups) run with one worker thread.
// A TXT answer too long for UDP comes back truncated and is asked again
// over TCP; that exchange waited in ppoll (poll on darwin) on the worker
// thread, so the program's own TCP server task could not run until the
// lookup timed out, twice (5 s each), and the lookup failed with "server
// misbehaving" (the dns stress on linux/arm64 with 64 tasks, 2026-11-16).
// Go's resolver parks the goroutine on the netpoller.
//
//   XO_DNSTCP_BACKENDS (arm64,llvm)

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

func TestDnsTcpOneThread(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the dns stress program")
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-dnstcp-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	for _, b := range envBackends("XO_DNSTCP_BACKENDS", []string{"arm64", "llvm"}) {
		bin := filepath.Join(work, "dns-stress-"+b)
		xb := testtmp.Guard(exec.Command(xo, "build", "--backend="+b, "-o", bin, filepath.Join(root, "std", "evidence", "dns", "stress")))
		xb.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
		if out, err := xb.CombinedOutput(); err != nil {
			t.Fatalf("xo build --backend=%s: %v\n%s", b, err, out)
		}
		for _, tasks := range []string{"8", "64"} {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			cmd := testtmp.Guard(exec.CommandContext(ctx, bin, "0", tasks))
			cmd.Env = append(os.Environ(), "XO_THREADS=1")
			start := time.Now()
			out, err := cmd.CombinedOutput()
			cancel()
			took := time.Since(start)
			if err != nil || !strings.Contains(string(out), " pass") {
				t.Fatalf("%s, XO_THREADS=1, %s tasks: %v after %v\n%s", b, tasks, err, took.Round(time.Millisecond), myfTail(string(out), 10))
			}
			// a TCP exchange that waited out a timeout takes 5 s
			if took > 4*time.Second {
				t.Errorf("%s, XO_THREADS=1, %s tasks: %v, a lookup waited out a timeout", b, tasks, took.Round(time.Millisecond))
			}
			t.Logf("%s, XO_THREADS=1, %s tasks: %v: %s", b, tasks, took.Round(time.Millisecond), strings.TrimSpace(myfTail(string(out), 1)))
		}
	}
}
