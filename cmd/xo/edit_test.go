package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const editProg = `fn load(id: Int) -> Int {
  id + 1
}

fn twice() -> Int {
  load(1) + load(2)
}
`

func TestEditCLI(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		code int
		want string // in the edited main.xo, or in stdout when rejected
	}{
		{"rename", []string{"rename", "load", "fetch"}, 0, "fn fetch(id: Int)"},
		{"change-sig", []string{"change-sig", "load", "(id: Int, step: Int = 1) -> Int"}, 0, "fn load(id: Int, step: Int = 1) -> Int"},
		{"rejected", []string{"change-sig", "load", "(id: Int, step: Int) -> Int"}, 1, `"ok": false`},
		{"dry run", []string{"--dry-run", "rename", "load", "fetch"}, 0, `"written": false`},
		// a dry run shows its changes as a diff (decision 0289)
		{"dry run diff", []string{"--dry-run", "rename", "load", "fetch"}, 0, `\n+fn fetch(id: Int) -> Int {\n`},
		{"unknown op", []string{"bogus"}, 2, ""},
		{"unknown name", []string{"rename", "nosuch", "x"}, 2, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := writeFiles(t, map[string]string{"main.xo": editProg})
			var out, errOut bytes.Buffer
			code := run(t, append([]string{"edit", "--dir=" + dir}, c.args...), &out, &errOut)
			if code != c.code {
				t.Fatalf("exit %d, want %d: %s %s", code, c.code, out.String(), errOut.String())
			}
			if c.want == "" {
				return
			}
			got := out.String()
			// a dry run's answer is its summary; otherwise the edited file
			if code == 0 && !strings.Contains(c.want, `"`) && c.args[0] != "--dry-run" {
				b, _ := os.ReadFile(filepath.Join(dir, "main.xo"))
				got = string(b)
				var res map[string]any
				if err := json.Unmarshal(out.Bytes(), &res); err != nil || res["ok"] != true {
					t.Errorf("summary = %s (%v)", out.String(), err)
				}
			}
			if !strings.Contains(got, c.want) {
				t.Errorf("want %q in:\n%s", c.want, got)
			}
		})
	}
}
