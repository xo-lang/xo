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
	src := "fn f(n: Int) -> Int {\n  if n > 0 {\n    return 1\n  }\n  else {\n    return 2\n  }\n}\nfn main(os: Os) {\n  os.stdio.println(\"${f(1)}\")\n}\n"
	dir := writeFiles(t, map[string]string{"app/main.xo": src})
	file := filepath.Join(dir, "app", "main.xo")
	var out, errOut bytes.Buffer
	if code := run([]string{"check", "--json", dir}, &out, &errOut); code != 1 {
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
	if code := run([]string{"check", "--fix", "--json", dir}, &out, &errOut); code != 0 {
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
	if code := run([]string{"check", "--fix", "--json", dir}, &out, &errOut); code != 0 || !strings.Contains(out.String(), `"applied": []`) {
		t.Errorf("second run: exit %d\n%s", code, out.String())
	}
	errOut.Reset()
	if code := run([]string{"check", "--fix", dir}, &out, &errOut); code != 0 || !strings.Contains(errOut.String(), "0 fix(es) applied") {
		t.Errorf("human: exit %d\n%s", code, errOut.String())
	}
}
