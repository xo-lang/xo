package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestBind runs `xo bind` (decision 0041): the binding lands in the cache
// under the project's xo.mod root, and `xo check` then uses it.
func TestBind(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs go list")
	}
	setenv(t, "XO_GOBIND_DIR", "")
	dir := writeFiles(t, map[string]string{
		"xo.mod":      "module ex\n",
		"app/main.xo": "use go \"strings\"\n\nfn main(os: Os) {\n  os.stdio.println(strings.to_upper(\"x\"))\n}\n",
	})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"bind", "--dir", filepath.Join(dir, "app"), "strings", "no/such/pkg"}, &out, &errOut); code != 1 {
		t.Fatalf("exit %d, want 1 (one package fails): %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "strings: functions") || !strings.Contains(errOut.String(), `cannot bind Go package "no/such/pkg"`) {
		t.Errorf("stdout %q stderr %q", out.String(), errOut.String())
	}
	if _, err := os.Stat(filepath.Join(dir, ".xo", "gobind", "strings.xo")); err != nil {
		t.Fatalf("binding not in the project cache: %v", err)
	}
	out.Reset()
	errOut.Reset()
	if code := run(t, []string{"bind", "--json", "--dir", dir, "strconv"}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var res []bindResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil || len(res) != 1 || res[0].Stats == nil || res[0].Stats.FuncsMapped == 0 || !res[0].Standard {
		t.Fatalf("json %s: %v", out.String(), err)
	}
	out.Reset()
	errOut.Reset()
	if code := run(t, []string{"check", filepath.Join(dir, "app")}, &out, &errOut); code != 0 {
		t.Fatalf("check exit %d: %s", code, errOut.String())
	}
}

// TestBindC runs `xo bind --c` (decision 0116): the binding of a header
// of the module lands in the project's .xo/cbind, with its coverage;
// `xo check` says `use c` needs --no-gc and `xo check --no-gc` accepts it.
func TestBindC(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("clang"); err != nil && os.Getenv("XO_CLANG") == "" {
		t.Skip("clang not found")
	}
	setenv(t, "XO_CBIND_DIR", "")
	dir := writeFiles(t, map[string]string{
		"xo.mod":      "module ex\n",
		"app/two.h":   "int two_add(int a, int b);\nvoid two_out(int *p);\n",
		"app/two.c":   "#include \"two.h\"\nint two_add(int a, int b) { return a + b; }\nvoid two_out(int *p) { *p = 2; }\n",
		"app/main.xo": "use c \"two.h\" link \"two.c\"\n\nfn main(os: Os) {\n  os.stdio.println(\"${two.two_add(1, 2)}\")\n}\n",
	})
	app := filepath.Join(dir, "app")
	var out, errOut bytes.Buffer
	if code := run(t, []string{"bind", "--c", "--dir", app, "--link", "two.c", "two.h", "no_such_xo.h"}, &out, &errOut); code != 1 {
		t.Fatalf("exit %d, want 1 (one header fails): %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "two.h (") || !strings.Contains(out.String(), "functions 1/2 bound, 1 skipped") || !strings.Contains(errOut.String(), `cannot bind C header "no_such_xo.h"`) {
		t.Errorf("stdout %q stderr %q", out.String(), errOut.String())
	}
	if ms, _ := filepath.Glob(filepath.Join(dir, ".xo", "cbind", "c_two_*.xo")); len(ms) != 1 {
		t.Fatalf("binding not in the project cache: %v", ms)
	}
	out.Reset()
	errOut.Reset()
	if code := run(t, []string{"bind", "--c", "--json", "--dir", app, "two.h"}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var res []cBindResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil || len(res) != 1 || res[0].Bound != 1 || res[0].Skipped["two_out"] == "" {
		t.Fatalf("json %s: %v", out.String(), err)
	}
	out.Reset()
	errOut.Reset()
	// `xo check` checks by the native backends' rules, as a build does, so
	// `use c` passes (owner, 2026-10-23, field report 0.26.0 b08); the Go
	// backend's rules (--backend=go) reject it
	if code := run(t, []string{"check", app}, &out, &errOut); code != 0 {
		t.Errorf("check: exit %d, want 0 (use c is valid natively)\n%s%s", code, out.String(), errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := run(t, []string{"check", "--backend=go", app}, &out, &errOut); code == 0 || !strings.Contains(out.String()+errOut.String(), "XO1201") {
		t.Errorf("check --backend=go: exit %d, want XO1201\n%s%s", code, out.String(), errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := run(t, []string{"check", "--no-gc", app}, &out, &errOut); code != 0 {
		t.Fatalf("check --no-gc exit %d: %s%s", code, out.String(), errOut.String())
	}
}

// TestUseGoPin covers decision 0356: a third-party `use go` without a
// `require go` line is XO1319, whose fix (xo fix) pins the version the
// binding resolved; `xo mod tidy` adds a missing pin too; Go's std needs
// none.
func TestUseGoPin(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs go list and fetches a Go module")
	}
	setenv(t, "XO_GOBIND_DIR", "")
	mod := "module ex\nedition preview\n"
	dir := writeFiles(t, map[string]string{
		"xo.mod":      mod,
		"app/main.xo": "use go \"strings\"\nuse go \"github.com/google/uuid\" as uuid\n\nfn main(os: Os) {\n  os.stdio.println(strings.to_upper(\"x\") + \"${uuid.new_string().len()}\")\n}\n",
	})
	app := filepath.Join(dir, "app")
	var out, errOut bytes.Buffer
	code := run(t, []string{"check", "--backend=go", app}, &out, &errOut)
	if strings.Contains(errOut.String(), "cannot bind") {
		t.Skipf("the Go module cannot be fetched here: %s", errOut.String())
	}
	if code != 1 || !strings.Contains(errOut.String(), "XO1319") || !strings.Contains(errOut.String(), "Go module github.com/google/uuid has no `require go` line") || strings.Contains(errOut.String(), `"strings"`) {
		t.Fatalf("check without the pin: exit %d\n%s", code, errOut.String())
	}
	out.Reset()
	errOut.Reset()
	run(t, []string{"fix", app}, &out, &errOut)
	data, _ := os.ReadFile(filepath.Join(dir, "xo.mod"))
	if !strings.Contains(string(data), "\nrequire go github.com/google/uuid v1.") {
		t.Fatalf("xo fix did not pin it: xo.mod\n%s\n%s%s", data, out.String(), errOut.String())
	}
	// once, and a second fix adds nothing: each round's check reads the
	// xo.mod the last round edited (Intel b12: one fix wrote the pin 25
	// times, once a round, as the check read xo.mod from disk)
	run(t, []string{"fix", app}, &out, &errOut)
	data, _ = os.ReadFile(filepath.Join(dir, "xo.mod"))
	if n := strings.Count(string(data), "require go github.com/google/uuid"); n != 1 {
		t.Fatalf("after two xo fix runs xo.mod pins the module %d times:\n%s", n, data)
	}
	// pinned, xo.lock has no go.sum hash for it yet (stage b): XO1305,
	// which tidy records
	out.Reset()
	errOut.Reset()
	if code := run(t, []string{"check", "--backend=go", app}, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "XO1305") || !strings.Contains(errOut.String(), "xo.lock has no hash for Go module github.com/google/uuid v1.") {
		t.Fatalf("check after the fix: exit %d\n%s", code, errOut.String())
	}
	if code := run(t, []string{"mod", "tidy", "--dir=" + dir}, &out, &errOut); code != 0 {
		t.Fatalf("tidy after the fix: exit %d\n%s", code, errOut.String())
	}
	lock, _ := os.ReadFile(filepath.Join(dir, "xo.lock"))
	if !strings.Contains(string(lock), "\ngo github.com/google/uuid v1.") || !strings.Contains(string(lock), "/go.mod h1:") {
		t.Fatalf("tidy wrote no Go lines: xo.lock\n%s", lock)
	}
	out.Reset()
	errOut.Reset()
	if code := run(t, []string{"check", "--backend=go", app}, &out, &errOut); code != 0 {
		t.Fatalf("check after tidy: exit %d\n%s", code, errOut.String())
	}
	// a changed hash is XO1304
	bad := strings.Replace(string(lock), "\ngo github.com/google/uuid v1.6.0 h1:", "\ngo github.com/google/uuid v1.6.0 h1:X", 1)
	if bad != string(lock) {
		os.WriteFile(filepath.Join(dir, "xo.lock"), []byte(bad), 0o644)
		out.Reset()
		errOut.Reset()
		if code := run(t, []string{"check", "--backend=go", app}, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "XO1304") {
			t.Fatalf("check with a changed hash: exit %d\n%s", code, errOut.String())
		}
		os.WriteFile(filepath.Join(dir, "xo.lock"), lock, 0o644)
	}
	// tidy adds the pin from the binding
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if code := run(t, []string{"mod", "tidy", "--dir=" + dir}, &out, &errOut); code != 0 {
		t.Fatalf("tidy: exit %d\n%s", code, errOut.String())
	}
	data, _ = os.ReadFile(filepath.Join(dir, "xo.mod"))
	if !strings.Contains(string(data), "require go github.com/google/uuid v1.") || strings.Contains(string(data), "require go strings") {
		t.Fatalf("tidy: xo.mod\n%s", data)
	}
}

// TestInlineStrWarnsOffArm64: --inline-str works on the native arm64
// backend only (decision 0370); a build where it does nothing says so
// instead of accepting it silently (Intel tester, 0.32.0), and an arm64
// build does not warn.
func TestInlineStrWarnsOffArm64(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{
		"xo.mod":  "module ex\n",
		"main.xo": "fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n",
	})
	for _, c := range []struct {
		backend string
		warn    bool
	}{{"amd64", true}, {"arm64", false}} {
		var out, errOut bytes.Buffer
		code := run(t, []string{"build", "--backend=" + c.backend, "--inline-str", "-o", filepath.Join(dir, "prog-"+c.backend), dir}, &out, &errOut)
		if code != 0 {
			t.Fatalf("build --backend=%s --inline-str: exit %d\n%s%s", c.backend, code, out.String(), errOut.String())
		}
		got := strings.Contains(errOut.String(), "--inline-str has no effect here")
		if got != c.warn {
			t.Errorf("build --backend=%s --inline-str: warned %v, want %v\n%s", c.backend, got, c.warn, errOut.String())
		}
	}
}

// TestUseGoTidyFreshClone: Intel b17 (0.33.0): in a fresh clone (no
// .xo/gobind) a `require go` pin has no go.sum hash to record, and tidy
// exited 0 without writing xo.lock's lines; now it is XO1320 and writes
// nothing.
func TestUseGoTidyFreshClone(t *testing.T) {
	t.Parallel()
	setenv(t, "XO_GOBIND_DIR", "")
	mod := "module example.com/b17\nedition preview\nrequire go github.com/google/uuid v1.6.0\n"
	dir := writeFiles(t, map[string]string{
		"xo.mod":  mod,
		"main.xo": "use go \"github.com/google/uuid\"\n\nfn main(os: Os) {\n  os.stdio.println(uuid.new_string())\n}\n",
	})
	var out, errOut bytes.Buffer
	code := run(t, []string{"mod", "tidy", "--dir=" + dir}, &out, &errOut)
	if code != 1 || !strings.Contains(errOut.String(), "XO1320") || !strings.Contains(errOut.String(), "run `xo check` once") {
		t.Fatalf("tidy in a fresh clone: exit %d\n%s%s", code, out.String(), errOut.String())
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "xo.mod")); string(data) != mod {
		t.Fatalf("tidy changed xo.mod:\n%s", data)
	}
	if lock, err := os.ReadFile(filepath.Join(dir, "xo.lock")); err == nil && strings.Contains(string(lock), "github.com/google/uuid") {
		t.Fatalf("tidy wrote the module's lines with no hashes known:\n%s", lock)
	}
}

// TestUseGoPatchGoLine: Intel b16 (0.33.0): golang.org/x/text v0.43.0's
// go.mod says `go 1.26.0`, above the generated `go 1.26` in Go's order,
// and go build stopped with "updates to go.mod needed"; the generated
// go line is now at least the binding workspace's.
func TestUseGoPatchGoLine(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs go list and fetches a Go module")
	}
	setenv(t, "XO_GOBIND_DIR", "")
	dir := writeFiles(t, map[string]string{
		"xo.mod":  "module example.com/b16\nedition preview\nrequire go golang.org/x/text v0.43.0\n",
		"main.xo": "use go \"golang.org/x/text/language\"\n\nfn main(os: Os) {\n  os.stdio.println(language.make(\"tr\").string())\n}\n",
	})
	var out, errOut bytes.Buffer
	run(t, []string{"check", "--backend=go", dir}, &out, &errOut)
	if strings.Contains(errOut.String(), "cannot bind") {
		t.Skipf("the Go module cannot be fetched here: %s", errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := run(t, []string{"mod", "tidy", "--dir=" + dir}, &out, &errOut); code != 0 {
		t.Fatalf("tidy: exit %d\n%s", code, errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := run(t, []string{"run", "--backend=go", dir}, &out, &errOut); code != 0 || strings.TrimSpace(out.String()) != "tr" {
		t.Fatalf("run: exit %d\nstdout %q\n%s", code, out.String(), errOut.String())
	}
}

// TestUseGoSharedType: Intel b14 (0.33.0): a Go type two bound packages
// use is one Xo type (decision 0392); net declared its own AddrPort,
// which netip.AddrPort could not be passed as (XO0401).
func TestUseGoSharedType(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("binds net and net/netip with go list")
	}
	setenv(t, "XO_GOBIND_DIR", "")
	dir := writeFiles(t, map[string]string{
		"xo.mod": "module example.com/b14\nedition preview\n",
		"main.xo": "use go \"net\"\nuse go \"net/netip\"\n\nfn main(os: Os) {\n" +
			"  let ap = netip.addr_port_from(netip.must_parse_addr(\"10.0.0.1\"), 80)\n" +
			"  match net.tcp_addr_from_addr_port(ap) {\n" +
			"    Some(t) => os.stdio.println(t.string())\n" +
			"    None => os.stdio.println(\"none\")\n  }\n}\n",
	})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"run", "--backend=go", dir}, &out, &errOut); code != 0 || strings.TrimSpace(out.String()) != "10.0.0.1:80" {
		t.Fatalf("run: exit %d\nstdout %q\n%s", code, out.String(), errOut.String())
	}
}

// TestUseGoNoGCOneErrorPerLine: `xo check --no-gc` of a `use go` program
// reports XO1201 once per `use go` line, not once per Go call (Windows
// tester, 0.33.0, bug 17: 45 errors for one program). Spec conformance
// (milestone 0.34.0); lands with the fix.
func TestUseGoNoGCOneErrorPerLine(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{
		"xo.mod": "module ex\nedition preview\n",
		"app/main.xo": "use go \"strings\"\nuse go \"strconv\"\n\nfn main(os: Os) {\n" +
			"  os.stdio.println(strings.to_upper(\"a\") + strings.to_lower(\"B\") + strings.repeat(\"c\", 2))\n" +
			"  os.stdio.println(strconv.itoa(1) + strconv.itoa(2) + strconv.quote(\"x\"))\n}\n",
	})
	var out, errOut bytes.Buffer
	code := run(t, []string{"check", "--no-gc", filepath.Join(dir, "app")}, &out, &errOut)
	if n := strings.Count(errOut.String(), "XO1201"); code != 1 || n != 2 {
		t.Fatalf("check --no-gc: exit %d, %d XO1201 (want 1 and 2, one per use go line):\n%s", code, n, errOut.String())
	}
}

// TestUseGoHelperFormatMismatch: an xo-gobind of another xo writes a
// binding format this xo does not read; the check says so (XO1101 naming
// the helper and both formats) instead of reporting a missing binding on
// every run (2026-10-27).
func TestUseGoHelperFormatMismatch(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("a shell script helper")
	}
	dir := writeFiles(t, map[string]string{
		"xo.mod":     "module ex\nedition preview\n",
		"main.xo":    "use go \"strings\"\n\nfn main(os: Os) {\n  os.stdio.println(strings.to_upper(\"a\"))\n}\n",
		"old-gobind": "#!/bin/sh\n# --dir ROOT PATH: a binding of format 1, as an old binder writes it\nmkdir -p \"$2/.xo/gobind\"\nprintf '// Code generated by xo bind from Go package strings. DO NOT EDIT.\\n// xo-bind: v1 go1.20\\n// go-package: strings\\n// go-standard: true\\n\\nuse std/go\\n' > \"$2/.xo/gobind/$3.xo\"\n",
	})
	helper := filepath.Join(dir, "old-gobind")
	if err := os.Chmod(helper, 0o755); err != nil {
		t.Fatal(err)
	}
	setenv(t, "XO_GOBIND_DIR", "")
	setenv(t, "XO_GOBIND", helper)
	var out, errOut bytes.Buffer
	code := run(t, []string{"check", "--backend=go", dir}, &out, &errOut)
	if code != 1 || !strings.Contains(errOut.String(), "XO1101") || !strings.Contains(errOut.String(), "xo-gobind at "+helper+" wrote binding format 1, this xo reads ") {
		t.Fatalf("check with an old helper: exit %d\n%s", code, errOut.String())
	}
}

// TestUseGoOfflineProxy: `use go` of real third-party modules with no
// network (owner, 2026-10-27, after the 0.33.0 testers' use-go bugs):
// testdata/goproxy is a GOPROXY file tree holding github.com/google/uuid
// v1.6.0 (a go.mod with no go line) and golang.org/x/sync v0.23.0 (a
// patch-level `go 1.26.0` line), copied from the module cache with their
// real metadata, read through a fresh GOMODCACHE with GOSUMDB off; xo.lock
// pins both with their real hashes.
func TestUseGoOfflineProxy(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs go list and go build")
	}
	proxy, err := filepath.Abs(filepath.Join("testdata", "goproxy"))
	if err != nil {
		t.Fatal(err)
	}
	// the module cache's files are read only: made writable before
	// t.TempDir removes it (cleanups run last registered first)
	modcache := t.TempDir()
	t.Cleanup(func() {
		filepath.WalkDir(modcache, func(p string, d os.DirEntry, err error) error {
			if err == nil {
				os.Chmod(p, 0o755)
			}
			return nil
		})
	})
	for k, v := range map[string]string{
		"GOPROXY": "file://" + filepath.ToSlash(proxy), "GOSUMDB": "off", "GOFLAGS": "-mod=mod",

		"GOMODCACHE": modcache, "GOTOOLCHAIN": "local", "GOPRIVATE": "", "GONOSUMDB": "", "GONOPROXY": "",
		"XO_GOBIND_DIR": "",
	} {
		setenv(t, k, v)
	}
	dir := writeFiles(t, map[string]string{
		"xo.mod": "module offline\n\nrequire go github.com/google/uuid v1.6.0\nrequire go golang.org/x/sync v0.23.0\n",
		"xo.lock": "go github.com/google/uuid v1.6.0 h1:NIvaJDMOsjHA8n1jAhLSgzrAzy1Hgr+hNrb57e+94F0=\n" +
			"go github.com/google/uuid v1.6.0/go.mod h1:TIyPZe4MgqvfeYDBFedMoGGpEw/LqOeaOT+nhxU+yHo=\n" +
			"go golang.org/x/sync v0.23.0 h1:KameEIfc1IkluZyXWLn39Wd4tURc6GbCiISGiZm2bQk=\n" +
			"go golang.org/x/sync v0.23.0/go.mod h1:sUUOizhqBxiL6pEWpqNLUiaJn1ShEbZ6BBqskPbjZm0=\n",
		"main.xo": "use go \"github.com/google/uuid\"\nuse go \"golang.org/x/sync/semaphore\"\n\n" +
			"fn main(os: Os) -> () ! Error {\n" +
			"  let id = uuid.parse(\"6ba7b810-9dad-11d1-80b4-00c04fd430c8\")?\n" +
			"  let s = semaphore.new_weighted(2)\n" +
			"  os.stdio.println(\"${id.string()} v${id.version()} ${s.try_acquire(2)} ${s.try_acquire(1)}\")\n" +
			"}\n",
	})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"run", "--backend=go", dir}, &out, &errOut); code != 0 {
		t.Fatalf("run: exit %d\n%s%s", code, out.String(), errOut.String())
	}
	if got, want := strings.TrimSpace(out.String()), "6ba7b810-9dad-11d1-80b4-00c04fd430c8 v1 true false"; got != want {
		t.Errorf("output %q, want %q", got, want)
	}
}

// TestGobindNewerGo (decision 0393): with an installed Go newer than the
// one the shipped xo-gobind was built with, xo builds the helper from the
// source a release carries under lib/xo, once, into the user cache, and
// uses that build; a second bind reuses it. A release layout of links
// (bin/xo, bin/xo-gobind, lib/xo/std and the helper's source) and a fake
// go command that says go1.99.0, reports the shipped helper as go1.26.1,
// logs each build of the helper (copying the shipped one), and passes
// everything else to the real go.
func TestGobindNewerGo(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list")
	}
	if runtime.GOOS == "windows" {
		t.Skip("fake go command is a shell script")
	}
	cli, err := xoCommand()
	if err != nil {
		t.Fatal(err)
	}
	realGo, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go command")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	rel := t.TempDir()
	for _, d := range []string{"bin", "lib/xo/cmd", "lib/xo/internal", "home"} {
		if err := os.MkdirAll(filepath.Join(rel, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	links := map[string]string{
		"lib/xo/std":             filepath.Join(root, "std"),
		"lib/xo/go.mod":          filepath.Join(root, "go.mod"),
		"lib/xo/cmd/xo-gobind":   filepath.Join(root, "cmd", "xo-gobind"),
		"lib/xo/internal/gobind": filepath.Join(root, "internal", "gobind"),
		"lib/xo/internal/gotool": filepath.Join(root, "internal", "gotool"),
	}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(rel, name)); err != nil {
			t.Fatal(err)
		}
	}
	// bin/xo a hard link (or a copy), not a symlink: xo finds std beside
	// the path it runs from after resolving links
	if err := os.Link(cli[0], filepath.Join(rel, "bin", "xo")); err != nil {
		data, err := os.ReadFile(cli[0])
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(rel, "bin", "xo"), data, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	shipped := filepath.Join(rel, "bin", "xo-gobind")
	build := exec.Command(realGo, "build", "-o", shipped, "./cmd/xo-gobind")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the shipped helper: %v\n%s", err, out)
	}
	log := filepath.Join(rel, "builds.log")
	fake := filepath.Join(rel, "go")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = env ] && [ \"$2\" = GOVERSION ]; then echo ${FAKE_GOVERSION:-go1.99.0}; exit 0; fi\n" +
		"if [ \"$1\" = version ] && [ $# -eq 2 ]; then echo \"$2: go1.26.1\"; exit 0; fi\n" +
		"if [ \"$1\" = build ] && [ \"$2\" = -o ] && [ \"$4\" = ./cmd/xo-gobind ]; then pwd -P >> '" + log + "'; cp '" + shipped + "' \"$3\"; exit $?; fi\n" +
		"exec '" + realGo + "' \"$@\"\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(rel, "home")
	for k, v := range map[string]string{"XO_GO": fake, "HOME": home, "XDG_CACHE_HOME": "", "XO_GOBIND": "", "XO_GOBIND_DIR": "", "XO_STD": "", cliEnv: ""} {
		setenv(t, k, v)
	}
	dir := writeFiles(t, map[string]string{"xo.mod": "module ex\n"})
	xo := []string{filepath.Join(rel, "bin", "xo")}
	for _, pkg := range []string{"strings", "strconv"} {
		var out, errOut bytes.Buffer
		if code := execCLI(t, xo, []string{"bind", "--dir", dir, pkg}, &out, &errOut); code != 0 {
			t.Fatalf("bind %s: exit %d: %s", pkg, code, errOut.String())
		}
	}
	data, _ := os.ReadFile(log)
	builds := strings.Split(strings.TrimSpace(string(data)), "\n")
	want, _ := filepath.EvalSymlinks(filepath.Join(rel, "lib", "xo"))
	if len(data) == 0 || len(builds) != 1 || builds[0] != want {
		t.Fatalf("helper builds %q, want one in %s/lib/xo", builds, rel)
	}
	got, _ := filepath.Glob(filepath.Join(home, "*", "*", "xo", "gobind", "*-go1.99.0", "xo-gobind"))
	more, _ := filepath.Glob(filepath.Join(home, "*", "xo", "gobind", "*-go1.99.0", "xo-gobind"))
	if len(got)+len(more) != 1 {
		t.Fatalf("cached helper %v %v, want one under %s", got, more, home)
	}
	// XO_GOBIND_LOCAL=1 builds it with an installed Go no newer than the
	// shipped helper's (the release smoke's case on a runner with that Go)
	setenv(t, "FAKE_GOVERSION", "go1.26.1")
	setenv(t, "XO_GOBIND_LOCAL", "1")
	var out, errOut bytes.Buffer
	if code := execCLI(t, xo, []string{"bind", "--dir", dir, "errors"}, &out, &errOut); code != 0 {
		t.Fatalf("bind errors with XO_GOBIND_LOCAL=1: exit %d: %s", code, errOut.String())
	}
	data, _ = os.ReadFile(log)
	if n := len(strings.Split(strings.TrimSpace(string(data)), "\n")); n != 2 {
		t.Fatalf("helper builds %d, want 2 (XO_GOBIND_LOCAL=1 builds one for go1.26.1)", n)
	}
}

// The h1 hashes of testdata/goproxy's synthetic example.com/top v1.0.0.
const (
	topSum    = "h1:mQgP8iM1qrNwUXzGFEMyuvgIhqc8aGeDqP53BfeWUOc="
	topModSum = "h1:9sH8E+n3/TzkC5xDbX0fMVl4CSZkfi4asaW3+Kn3Fcg="
)

// TestUseGoMissingIndirectRequire (the Windows tester's bug 20,
// 2026-10-29): a binding workspace whose go.mod lacks a transitive
// `// indirect` requirement (bindings made one at a time left them out)
// still builds: go build runs with -mod=mod, so the go command adds the
// requirement from the module cache. testdata/goproxy's example.com/top
// imports example.com/leaf (synthetic modules); the test binds top, drops
// leaf's line from the workspace's go.mod, and builds again with
// GOFLAGS empty (TestUseGoOfflineProxy sets -mod=mod there).
func TestUseGoMissingIndirectRequire(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs go list and go build")
	}
	proxy, err := filepath.Abs(filepath.Join("testdata", "goproxy"))
	if err != nil {
		t.Fatal(err)
	}
	modcache := t.TempDir()
	t.Cleanup(func() {
		filepath.WalkDir(modcache, func(p string, d os.DirEntry, err error) error {
			if err == nil {
				os.Chmod(p, 0o755)
			}
			return nil
		})
	})
	for k, v := range map[string]string{
		"GOPROXY": "file://" + filepath.ToSlash(proxy), "GOSUMDB": "off", "GOFLAGS": "",
		"GOMODCACHE": modcache, "GOTOOLCHAIN": "local", "GOPRIVATE": "", "GONOSUMDB": "", "GONOPROXY": "",
		"XO_GOBIND_DIR": "",
	} {
		setenv(t, k, v)
	}
	dir := writeFiles(t, map[string]string{
		"xo.mod": "module indirect\n\nrequire go example.com/top v1.0.0\n",
		"xo.lock": "go example.com/top v1.0.0 " + topSum + "\n" +
			"go example.com/top v1.0.0/go.mod " + topModSum + "\n",
		"main.xo": "use go \"example.com/top\"\n\nfn main(os: Os) {\n  os.stdio.println(\"${top.quad(5)}\")\n}\n",
	})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"run", "--backend=go", dir}, &out, &errOut); code != 0 || strings.TrimSpace(out.String()) != "20" {
		t.Fatalf("first run: exit %d\n%s%s", code, out.String(), errOut.String())
	}
	gm := filepath.Join(dir, ".xo", "gobind", "_mod", "go.mod")
	b, err := os.ReadFile(gm)
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	dropped := false
	for _, l := range strings.Split(string(b), "\n") {
		if strings.Contains(l, "example.com/leaf") {
			dropped = true
			continue
		}
		kept = append(kept, l)
	}
	if !dropped {
		t.Fatalf("the workspace's go.mod has no example.com/leaf line to drop:\n%s", b)
	}
	if err := os.WriteFile(gm, []byte(strings.Join(kept, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if code := run(t, []string{"build", "--backend=go", "-o", filepath.Join(dir, "app"), dir}, &out, &errOut); code != 0 {
		t.Fatalf("build without leaf's requirement: exit %d\n%s%s", code, out.String(), errOut.String())
	}
}

// TestUseGoNestedModulePin: a pin of an enclosing Go module does not pin
// a package in a nested module of its own. testdata/goproxy's synthetic
// example.com/nest/sub v1.2.0 is a module inside example.com/nest's path
// (as aws-sdk-go-v2's service/s3 is, the Go workflows suite, 2026-10-29).
// With only `require go example.com/nest v1.0.0`, binding sub fetched
// example.com/nest/sub@v1.0.0, which does not exist; now sub binds at its
// own version, check asks for its pin (XO1319), tidy adds it, and the
// program runs.
func TestUseGoNestedModulePin(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs go list and go build")
	}
	proxy, err := filepath.Abs(filepath.Join("testdata", "goproxy"))
	if err != nil {
		t.Fatal(err)
	}
	modcache := t.TempDir()
	t.Cleanup(func() {
		filepath.WalkDir(modcache, func(p string, d os.DirEntry, err error) error {
			if err == nil {
				os.Chmod(p, 0o755)
			}
			return nil
		})
	})
	for k, v := range map[string]string{
		"GOPROXY": "file://" + filepath.ToSlash(proxy), "GOSUMDB": "off", "GOFLAGS": "-mod=mod",
		"GOMODCACHE": modcache, "GOTOOLCHAIN": "local", "GOPRIVATE": "", "GONOSUMDB": "", "GONOPROXY": "",
		"XO_GOBIND_DIR": "",
	} {
		setenv(t, k, v)
	}
	dir := writeFiles(t, map[string]string{
		"xo.mod":  "module nested\nedition preview\nrequire go example.com/nest v1.0.0\n",
		"main.xo": "use go \"example.com/nest/sub\"\n\nfn main(os: Os) {\n  os.stdio.println(\"${sub.two()}\")\n}\n",
	})
	var out, errOut bytes.Buffer
	code := run(t, []string{"check", dir}, &out, &errOut)
	if code != 1 || !strings.Contains(errOut.String(), "XO1319") || !strings.Contains(errOut.String(), "example.com/nest/sub v1.2.0") {
		t.Fatalf("check: exit %d, want XO1319 for example.com/nest/sub v1.2.0\n%s%s", code, out.String(), errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := run(t, []string{"mod", "tidy", "--dir=" + dir}, &out, &errOut); code != 0 {
		t.Fatalf("tidy: exit %d\n%s%s", code, out.String(), errOut.String())
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "xo.mod")); !strings.Contains(string(data), "require go example.com/nest/sub v1.2.0") {
		t.Fatalf("tidy did not pin the nested module:\n%s", data)
	}
	out.Reset()
	errOut.Reset()
	if code := run(t, []string{"run", "--backend=go", dir}, &out, &errOut); code != 0 || strings.TrimSpace(out.String()) != "2" {
		t.Fatalf("run: exit %d\n%s%s", code, out.String(), errOut.String())
	}
}
