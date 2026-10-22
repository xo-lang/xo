package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// lockedBuf is a bytes.Buffer safe to read while a command writes it.
type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// xoCmd is the command line of the xo the tests run, with args.
func xoCmd(t *testing.T, args ...string) []string {
	t.Helper()
	cli, err := xoCommand()
	if err != nil {
		t.Fatalf("no xo: %v", err)
	}
	return append(append([]string{}, cli...), args...)
}

// The 0.25.2 field report of 2026-10-22: each test reproduces one item.

func xoOut(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(t, args, &out, &errb)
	return code, out.String() + errb.String()
}

// TestFieldMissingStdImport: `json.encode` without `use std/json` (but
// with std/http, which has a member json) offers the import first, with
// a fix that adds it.
func TestFieldMissingStdImport(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{
		"app/xo.mod":  "module app\n",
		"app/main.xo": "use std/http\n\nfn main(os: Os) {\n  os.stdio.println(\"${json.encode([1]).len()}\")\n  let _r = http.empty(200)\n}\n",
	})
	app := filepath.Join(dir, "app")
	code, out := xoOut(t, "check", app)
	i := strings.Index(out, "add `use std/json`")
	j := strings.Index(out, "refer to it as `http.json`")
	if code != 1 || i < 0 || (j >= 0 && j < i) {
		t.Fatalf("check: exit %d, want the import note first\n%s", code, out)
	}
	if code, out := xoOut(t, "check", "--fix", app); code != 0 {
		t.Fatalf("check --fix: exit %d\n%s", code, out)
	}
	if b, _ := os.ReadFile(filepath.Join(app, "main.xo")); !strings.Contains(string(b), "use std/http\nuse std/json\n") {
		t.Errorf("not fixed:\n%s", b)
	}
}

// TestFieldQueryHolesDir: `xo query holes .` takes the directory.
func TestFieldQueryHolesDir(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{
		"app/xo.mod":  "module app\n",
		"app/main.xo": "fn f() -> Int { ??? }\n\nfn main(os: Os) {\n  os.stdio.println(\"${f()}\")\n}\n",
	})
	code, out := xoOut(t, "query", "holes", "--json", filepath.Join(dir, "app"))
	if code != 0 || !strings.Contains(out, "main.xo") {
		t.Errorf("query holes DIR: exit %d\n%s", code, out)
	}
}

// TestFieldAddFieldPub: add-field on a struct whose fields are all pub
// adds a pub field.
func TestFieldAddFieldPub(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{
		"app/xo.mod":  "module app\n",
		"app/main.xo": "pub struct User {\n  pub id: Int\n  pub name: Str\n}\n\nfn main(os: Os) {\n  os.stdio.println(User{id: 1, name: \"a\"}.name)\n}\n",
	})
	app := filepath.Join(dir, "app")
	if code, out := xoOut(t, "edit", "--dir="+app, "add-field", "User", "email: Str = \"\""); code != 0 {
		t.Fatalf("edit: exit %d\n%s", code, out)
	}
	if b, _ := os.ReadFile(filepath.Join(app, "main.xo")); !strings.Contains(string(b), "  pub email: Str\n") {
		t.Errorf("the new field is not pub:\n%s", b)
	}
}

// TestFieldWarningsInScope: `xo build .` prints its own file's warning
// and does not count it as outside the directories built.
func TestFieldWarningsInScope(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{
		"app/xo.mod":            "module app\n",
		"app/static/index.html": "<html></html>\n",
		"app/main.xo":           "use std/embed\n\nconst SITE: embed.Dir = embed(\"static\")\n\nfn main(os: Os) {\n  let b = SITE.read(\"index.html\") ?? b\"\"\n  os.stdio.println(\"${b.len()}\")\n}\n",
	})
	w := xoCmd(t, "build", "-o", filepath.Join(t.TempDir(), "app"), ".")
	cmd := command(t, w[0], w[1:]...)
	cmd.Dir = filepath.Join(dir, "app")
	b, _ := cmd.CombinedOutput()
	out := string(b)
	if strings.Contains(out, "outside the directories built") || !strings.Contains(out, "XO1511") {
		t.Errorf("xo build .:\n%s", out)
	}
}

// TestFieldEnsuresResult: a failed ensures whose condition reads result
// inside a call or method prints result's value, on every backend.
func TestFieldEnsuresResult(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds programs")
	}
	dir := writeFiles(t, map[string]string{
		"app/xo.mod":  "module app\n",
		"app/main.xo": "fn ok(s: Str) -> Bool { s.len() < 3 }\n\nfn name(n: Int) -> Str\n  ensures(ok(result))\n{\n  \"x\".repeat(n)\n}\n\nfn main(os: Os) {\n  os.stdio.println(name(5))\n}\n",
	})
	app := filepath.Join(dir, "app")
	for _, be := range append([]string{"go"}, nativeBackends(t)...) {
		if code, out := xoOut(t, "run", "--backend="+be, app); code == 0 || !strings.Contains(out, `result = "xxxxx"`) {
			t.Errorf("%s: exit %d, no result value\n%s", be, code, out)
		}
	}
}

// TestFieldWasiNetGo: the Go backend refuses a network program for
// wasip1/wasm, as the wasm backend does.
func TestFieldWasiNetGo(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs the Go generator")
	}
	dir := writeFiles(t, map[string]string{
		"app/xo.mod":  "module app\n",
		"app/main.xo": "fn main(os: Os) ! {\n  let l = os.net.listen(\":0\")?\n  os.stdio.println(l.addr())\n}\n",
	})
	code, out := xoOut(t, "build", "--backend=go", "--targets=wasip1/wasm", "-o", filepath.Join(t.TempDir(), "x.wasm"), filepath.Join(dir, "app"))
	if code == 0 || !strings.Contains(out, "WASI preview1 has no sockets") {
		t.Errorf("exit %d\n%s", code, out)
	}
}

// TestFieldRunForwardsStop: `xo run` with standard input not a terminal
// sends a stop request it gets on to the program, which then ends
// through its own stop handling.
func TestFieldRunForwardsStop(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	dir := writeFiles(t, map[string]string{
		"app/xo.mod":  "module app\n",
		"app/main.xo": "use std/task\n\nfn main(os: Os) {\n  os.stdio.println(\"ready\")\n  let _ = task.stopping().recv()\n  os.stdio.println(\"stopped cleanly\")\n}\n",
	})
	w := xoCmd(t, "run", filepath.Join(dir, "app"))
	cmd := command(t, w[0], w[1:]...)
	var out lockedBuf
	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Minute)
	for !strings.Contains(out.String(), "ready") && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		cmd.Process.Kill()
		t.Fatalf("xo run did not end after SIGTERM\n%s", out.String())
	}
	if !strings.Contains(out.String(), "stopped cleanly") {
		t.Errorf("the program did not see the stop\n%s", out.String())
	}
}
