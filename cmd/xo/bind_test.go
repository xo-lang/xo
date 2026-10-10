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
	if testing.Short() {
		t.Skip("runs go list")
	}
	t.Setenv("XO_GOBIND_DIR", "")
	dir := writeFiles(t, map[string]string{
		"xo.mod":      "module ex\n",
		"app/main.xo": "use go \"strings\"\n\nfn main(os: Os) {\n  os.stdio.println(strings.to_upper(\"x\"))\n}\n",
	})
	var out, errOut bytes.Buffer
	if code := run([]string{"bind", "--dir", filepath.Join(dir, "app"), "strings", "no/such/pkg"}, &out, &errOut); code != 1 {
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
	if code := run([]string{"bind", "--json", "--dir", dir, "strconv"}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var res []bindResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil || len(res) != 1 || res[0].Stats == nil || res[0].Stats.FuncsMapped == 0 || !res[0].Standard {
		t.Fatalf("json %s: %v", out.String(), err)
	}
	out.Reset()
	errOut.Reset()
	if code := run([]string{"check", filepath.Join(dir, "app")}, &out, &errOut); code != 0 {
		t.Fatalf("check exit %d: %s", code, errOut.String())
	}
}

// TestBindC runs `xo bind --c` (decision 0116): the binding of a header
// of the module lands in the project's .xo/cbind, with its coverage;
// `xo check` says `use c` needs --no-gc and `xo check --no-gc` accepts it.
func TestBindC(t *testing.T) {
	if _, err := exec.LookPath("clang"); err != nil && os.Getenv("XO_CLANG") == "" {
		t.Skip("clang not found")
	}
	t.Setenv("XO_CBIND_DIR", "")
	dir := writeFiles(t, map[string]string{
		"xo.mod":      "module ex\n",
		"app/two.h":   "int two_add(int a, int b);\nvoid two_out(int *p);\n",
		"app/two.c":   "#include \"two.h\"\nint two_add(int a, int b) { return a + b; }\nvoid two_out(int *p) { *p = 2; }\n",
		"app/main.xo": "use c \"two.h\" link \"two.c\"\n\nfn main(os: Os) {\n  os.stdio.println(\"${two.two_add(1, 2)}\")\n}\n",
	})
	app := filepath.Join(dir, "app")
	var out, errOut bytes.Buffer
	if code := run([]string{"bind", "--c", "--dir", app, "--link", "two.c", "two.h", "no_such_xo.h"}, &out, &errOut); code != 1 {
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
	if code := run([]string{"bind", "--c", "--json", "--dir", app, "two.h"}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var res []cBindResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil || len(res) != 1 || res[0].Bound != 1 || res[0].Skipped["two_out"] == "" {
		t.Fatalf("json %s: %v", out.String(), err)
	}
	out.Reset()
	errOut.Reset()
	if code := run([]string{"check", app}, &out, &errOut); code == 0 || !strings.Contains(out.String()+errOut.String(), "XO1201") {
		t.Errorf("check without --no-gc: exit %d\n%s%s", code, out.String(), errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := run([]string{"check", "--no-gc", app}, &out, &errOut); code != 0 {
		t.Fatalf("check --no-gc exit %d: %s%s", code, out.String(), errOut.String())
	}
}
