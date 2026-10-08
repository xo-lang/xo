package main

import (
	"bytes"
	"encoding/json"
	"os"
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
