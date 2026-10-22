package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCheckFix covers `xo check --fix` (decision 0096): plain check leaves
// the file alone, --fix applies the machine fixes, reformats, and lists
// them under "applied".
func TestCheckFix(t *testing.T) {
	t.Parallel()
	src := "fn f(n: Int) -> Int {\n  if n > 0 {\n    return 1\n  }\n  else {\n    return 2\n  }\n}\nfn main(os: Os) {\n  os.stdio.println(\"${f(1)}\")\n}\n"
	dir := writeFiles(t, map[string]string{"app/main.xo": src})
	file := filepath.Join(dir, "app", "main.xo")
	var out, errOut bytes.Buffer
	if code := run(t, []string{"check", "--json", dir + "/..."}, &out, &errOut); code != 1 {
		t.Fatalf("plain check: exit %d, want 1: %s%s", code, out.String(), errOut.String())
	}
	if strings.Contains(out.String(), `"applied"`) {
		t.Errorf("plain check lists applied fixes:\n%s", out.String())
	}
	if b, _ := os.ReadFile(file); string(b) != src {
		t.Fatalf("plain check changed the file:\n%s", b)
	}
	out.Reset()
	errOut.Reset()
	if code := run(t, []string{"check", "--fix", "--json", dir + "/..."}, &out, &errOut); code != 0 {
		t.Fatalf("check --fix: exit %d: %s%s", code, out.String(), errOut.String())
	}
	var res struct {
		Diagnostics []json.RawMessage `json:"diagnostics"`
		Applied     []struct {
			File string `json:"file"`
			Line int    `json:"line"`
			Rule string `json:"rule"`
		} `json:"applied"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out.String())
	}
	if len(res.Diagnostics) != 0 || len(res.Applied) != 1 || res.Applied[0].Rule != "XO0162" {
		t.Errorf("result:\n%s", out.String())
	}
	if b, _ := os.ReadFile(file); !strings.Contains(string(b), "} else {") {
		t.Errorf("file not fixed:\n%s", b)
	}
	// Nothing left to fix: an empty list, not an absent one.
	out.Reset()
	if code := run(t, []string{"check", "--fix", "--json", dir + "/..."}, &out, &errOut); code != 0 || !strings.Contains(out.String(), `"applied": []`) {
		t.Errorf("second run: exit %d\n%s", code, out.String())
	}
	errOut.Reset()
	if code := run(t, []string{"check", "--fix", dir + "/..."}, &out, &errOut); code != 0 || !strings.Contains(errOut.String(), "0 fix(es) applied") {
		t.Errorf("human: exit %d\n%s", code, errOut.String())
	}
}

// TestCheckWIP covers `xo check --wip` (decision 0293): an unused binding
// (XO0110) and an unused computed value (XO0209) are warnings with a note,
// so the check passes, while a plain check still rejects them and any
// other error still fails --wip.
func TestCheckWIP(t *testing.T) {
	t.Parallel()
	src := "fn f(n: Int) -> Int {\n  let unused = n * 2\n  n.abs()\n  n\n}\nfn main(os: Os) {\n  os.stdio.println(\"${f(1)}\")\n}\n"
	dir := writeFiles(t, map[string]string{"app/main.xo": src})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"check", dir + "/..."}, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "error XO0110") || !strings.Contains(errOut.String(), "error XO0209") {
		t.Fatalf("plain check: exit %d, want 1 with XO0110 and XO0209 errors:\n%s", code, errOut.String())
	}
	errOut.Reset()
	if code := run(t, []string{"check", "--wip", dir + "/..."}, &out, &errOut); code != 0 {
		t.Fatalf("check --wip: exit %d, want 0:\n%s", code, errOut.String())
	}
	for _, want := range []string{"warning XO0110", "warning XO0209", "note: an error outside `xo check --wip`", "0 error(s), 2 warning(s)"} {
		if !strings.Contains(errOut.String(), want) {
			t.Errorf("check --wip output lacks %q:\n%s", want, errOut.String())
		}
	}
	out.Reset()
	if code := run(t, []string{"check", "--wip", "--json", dir + "/..."}, &out, &errOut); code != 0 || strings.Contains(out.String(), `"severity": "error"`) || !strings.Contains(out.String(), `"severity": "warning"`) {
		t.Errorf("check --wip --json: exit %d\n%s", code, out.String())
	}
	bad := writeFiles(t, map[string]string{"app/main.xo": "fn main(os: Os) {\n  let unused = 1\n  os.stdio.println(missing)\n}\n"})
	errOut.Reset()
	if code := run(t, []string{"check", "--wip", bad + "/..."}, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "error XO0601") || !strings.Contains(errOut.String(), "warning XO0110") {
		t.Errorf("check --wip with an unresolved name: exit %d, want 1:\n%s", code, errOut.String())
	}
}
