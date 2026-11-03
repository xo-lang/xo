package selfhost

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestWasmCommandStdinParks runs a wasip2/wasm command whose one task
// reads a line of standard input while another prints on a timer, with
// the input written 3 seconds after the start: the ticks come first, so
// the read parked only its task on the stdin stream's pollable (rt9.xo
// $wasi_stdin_wait_read). A blocking-read stopped every task until the
// line came (rt7.xo, gotchas/wasm-stdin-eagain.md), so "read: hello"
// printed before the ticks. Needs wasmtime (needWasmTools).
func TestWasmCommandStdinParks(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	needWasmTools(t, "wasmtime")
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root := repoRoot(t)
	const src = `fn main(os: Os) {
  let out = os.stdio
  let clock = os.clock
  scope s {
    s.spawn(fn() {
      for i in 0..5 {
        clock.sleep(20ms)
        out.println("tick ${i}")
      }
      out.println("ticks done")
    })
    s.spawn(fn() {
      match out.read_line() {
        Ok(Some(l)) => out.println("read: ${l}")
        Ok(None) => out.println("read: end")
        Err(e) => out.println("read: ${e}")
      }
    })
  }
}
`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module m\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	comp := filepath.Join(t.TempDir(), "m.wasm")
	build := testtmp.Guard(exec.Command(xo, "build", "--targets=wasip2/wasm", "-o", comp, "."))
	build.Dir = dir
	build.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "XO_BIN_CACHE=off")
	if b, err := build.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, b)
	}
	cmd := testtmp.Guard(exec.Command("wasmtime", "run", "-C", "cache=n", comp))
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * time.Second)
	if _, err := in.Write([]byte("hello\n")); err != nil {
		t.Fatal(err)
	}
	in.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
	case <-time.After(60 * time.Second):
		cmd.Process.Kill()
		t.Fatalf("no exit in 60s; stdout so far:\n%s", stdout.String())
	}
	if err != nil {
		t.Fatalf("%v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	want := "tick 0\ntick 1\ntick 2\ntick 3\ntick 4\nticks done\nread: hello\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout:\n%s\nwant:\n%s(the ticks before the read: the read parks only its task)", got, want)
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr:\n%s", stderr.String())
	}
}
