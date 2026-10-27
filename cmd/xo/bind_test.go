package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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
