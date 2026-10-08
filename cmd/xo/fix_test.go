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

	for _, bad := range [][]string{{"fix", "--from=draft-9"}, {"fix", "--edition=2031"}} {
		if code := run(bad, &out, &errOut); code != 2 {
			t.Errorf("%v: exit %d, want 2", bad, code)
		}
	}
}
