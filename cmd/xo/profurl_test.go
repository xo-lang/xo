package main

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// profURLProg is a server with std/debug mounted at /debug and a fiber
// that burns CPU, so a one second profile has samples.
const profURLProg = `use std/debug
use std/http

fn burn() -> Int {
  var x = 1
  var i = 0
  while i < 400000000 {
    x = (x * 31 + i) % 1000003
    i += 1
  }
  x
}

fn main(os: Os) ! {
  let port = os.env.get("PORT") ?? "0"
  var r = http.router()
  r.mount("/debug", debug.router(os.clock, fn(_req) { true }))
  let srv = http.server(os.net.listen("127.0.0.1:${port}")?, r)
  scope t {
    let b = t.spawn(burn)
    os.stdio.println("ready")
    srv.serve()?
    let _r = b.await
  }
}
`

// TestProfURL: xo prof URL (decision 0325) fetches a running server's
// profile from its std/debug endpoint and converts it like xo prof FILE;
// a text answer (/debug/sched) is printed; a refusal is an error naming
// the status.
func TestProfURL(t *testing.T) {
	setenv(t, "XO_BUILD_DIR", filepath.Join(t.TempDir(), "build"))
	dir := writeFiles(t, map[string]string{"main.xo": profURLProg})
	res, err := xotool.Build([]string{dir}, xotool.Options{Backend: nativeBackends(t)[0], Out: filepath.Join(t.TempDir(), "srv")})
	if err != nil || res.Failed {
		t.Fatalf("build: %v %s", err, res.Output)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	cmd := command(t, res.Binaries[xotool.Host()])
	testtmp.Lifeline(t, cmd, 10*time.Minute)
	cmd.Env = append(cmd.Env, fmt.Sprintf("PORT=%d", port))
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	base := fmt.Sprintf("http://127.0.0.1:%d/debug", port)
	deadline := time.Now().Add(20 * time.Second)
	for {
		c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			c.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server did not listen: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	work := t.TempDir()
	raw := filepath.Join(work, "p.xoraw")
	var out, errOut bytes.Buffer
	code := run(t, []string{"prof", base + "/profile?seconds=1&hz=500", "--raw-out=" + raw, "--cpu-out=" + filepath.Join(work, "p.cpu.pprof")}, &out, &errOut)
	if code != 0 {
		t.Fatalf("prof URL exit %d: %s", code, errOut.String())
	}
	for _, f := range []string{raw, filepath.Join(work, "p.cpu.pprof"), filepath.Join(work, "p.cpu.txt")} {
		if st, err := os.Stat(f); err != nil || st.Size() == 0 {
			t.Errorf("%s missing or empty: %v", f, err)
		}
	}
	if txt, _ := os.ReadFile(filepath.Join(work, "p.cpu.txt")); !strings.Contains(string(txt), "burn") {
		t.Errorf("the summary does not name burn:\n%s", txt)
	}

	out.Reset()
	errOut.Reset()
	if code := run(t, []string{"prof", base + "/sched"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "\n") {
		t.Errorf("prof sched: exit %d out %q err %q", code, out.String(), errOut.String())
	}

	out.Reset()
	errOut.Reset()
	code = run(t, []string{"prof", base + "/nothing", "--raw-out=" + filepath.Join(work, "n.xoraw")}, &out, &errOut)
	if code != 1 || !strings.Contains(errOut.String(), "404") {
		t.Errorf("prof missing route: exit %d err %q", code, errOut.String())
	}
}
