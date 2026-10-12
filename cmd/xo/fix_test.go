package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFixCommand(t *testing.T) {
	src := "fn f(n: Int) -> Int ! Str {\n  if n > 0 { return ok(n) }\n  err(\"bad {n}\")\n}\n"
	dir := writeFiles(t, map[string]string{"m/a.xo": src})
	mod := filepath.Join(dir, "m")

	var out, errOut bytes.Buffer
	if code := run([]string{"fix", "--dry-run", "--json", "--from=draft-0.1", mod}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var res struct {
		DryRun    bool              `json:"dry_run"`
		Files     []string          `json:"files"`
		Changes   []json.RawMessage `json:"changes"`
		Remaining []json.RawMessage `json:"remaining"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	if !res.DryRun || len(res.Files) != 1 || len(res.Changes) != 3 || len(res.Remaining) != 0 {
		t.Errorf("json = %s", out.String())
	}
	if b, _ := os.ReadFile(filepath.Join(mod, "a.xo")); string(b) != src {
		t.Errorf("dry run changed the file")
	}

	out.Reset()
	if code := run([]string{"fix", "--from=draft-0.1", mod}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "3 change(s), changed 1 file(s)") {
		t.Errorf("output:\n%s", out.String())
	}
	if b, _ := os.ReadFile(filepath.Join(mod, "a.xo")); !strings.Contains(string(b), `Err("bad ${n}")`) {
		t.Errorf("a.xo = %s", b)
	}

	for _, bad := range [][]string{{"fix", "--from=draft-9"}, {"fix", "--edition=2031"}, {"fix", "--edition=2027"}} {
		if code := run(bad, &out, &errOut); code != 2 {
			t.Errorf("%v: exit %d, want 2", bad, code)
		}
	}
}

// TestFixEditionFlag checks `xo fix --edition=preview` (and the old name
// 2026, an alias): `edition 2026` in xo.mod becomes `edition preview`
// (XO1315, decision 0155).
func TestFixEditionFlag(t *testing.T) {
	for _, flag := range []string{"--edition=preview", "--edition=2026", ""} {
		dir := writeFiles(t, map[string]string{"xo.mod": "module example.com/a\nedition 2026\n", "main.xo": "fn main() {\n}\n"})
		args := []string{"fix", dir}
		if flag != "" {
			args = []string{"fix", flag, dir}
		}
		var out, errOut bytes.Buffer
		if code := run(args, &out, &errOut); code != 0 {
			t.Fatalf("%v: exit %d: %s", args, code, errOut.String())
		}
		if b, _ := os.ReadFile(filepath.Join(dir, "xo.mod")); string(b) != "module example.com/a\nedition preview\n" {
			t.Errorf("%v: xo.mod = %q", args, b)
		}
		if !strings.Contains(out.String(), "XO1315") || !strings.Contains(out.String(), "1 change(s), changed 1 file(s)") {
			t.Errorf("%v: output:\n%s", args, out.String())
		}
	}
}

// TestWorkEdition checks the `xo <edition>` line of xo.work through the
// CLI: `xo 2026` builds with the fixable note XO1315 and `xo fix` rewrites
// it to `xo preview`; an unknown edition is XO1301 (decision 0155).
func TestWorkEdition(t *testing.T) {
	t.Setenv("XO_WORK", "")
	files := func(edition string) string {
		return writeFiles(t, map[string]string{
			"xo.work":   "xo " + edition + "\nuse ./a\n",
			"a/xo.mod":  "module example.com/a\nedition preview\n",
			"a/main.xo": "fn main() {\n}\n",
		})
	}
	ws := files("2026")
	app := filepath.Join(ws, "a")
	r := xo(t, "check", "--json", app).want(0)
	if got := diagCodes(t, r.out); got != "XO1315" {
		t.Errorf("xo 2026: check codes %q, want XO1315", got)
	}
	if !strings.Contains(r.out, "`xo 2026` is the old name of `xo preview`") {
		t.Errorf("xo 2026: message:\n%s", r.out)
	}
	r = xo(t, "fix", app).want(0)
	if !strings.Contains(r.out, "XO1315") || !strings.Contains(r.out, "1 change(s), changed 1 file(s)") {
		t.Errorf("xo fix output:\n%s", r.out)
	}
	if b, _ := os.ReadFile(filepath.Join(ws, "xo.work")); string(b) != "xo preview\nuse ./a\n" {
		t.Errorf("xo.work after fix = %q", b)
	}
	r = xo(t, "check", "--json", app).want(0)
	if got := diagCodes(t, r.out); got != "" {
		t.Errorf("after fix: check codes %q", got)
	}

	ws = files("2027")
	r = xo(t, "check", "--json", filepath.Join(ws, "a")).want(1)
	if got := diagCodes(t, r.out); got != "XO1301" || !strings.Contains(r.out, `edition \"2027\" is not supported`) {
		t.Errorf("xo 2027: check codes %q:\n%s", got, r.out)
	}
}
