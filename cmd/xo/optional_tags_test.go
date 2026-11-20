package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestGoBackendOptionalTags (task #429 lever C): a Go backend program is
// built with xo_no_M for each std/go_optional.txt module M it does not
// reach, so std/rt's Go for M, and the Go packages it imports (whose init
// runs at startup), are left out; one that reaches std/template keeps
// it. The tags are read from the binary's build information.
func TestGoBackendOptionalTags(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go build")
	}
	progs := map[string]string{
		"plain": "fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n",
		"tmpl":  "use std/template\n\nfn main(os: Os) {\n  match template.parse(\"t\", \"x\") {\n    Ok(_) => os.stdio.println(\"ok\")\n    Err(e) => os.stdio.println(e.display())\n  }\n}\n",
	}
	want := map[string][2][]string{
		"plain": {{"xo_no_template", "xo_no_xml", "xo_no_archive", "xo_no_gif", "xo_nohttp"}, nil},
		"tmpl":  {{"xo_no_xml", "xo_no_archive", "xo_no_gif"}, {"xo_no_template"}},
	}
	re := regexp.MustCompile(`(?m)^\s*build\s+-tags=(\S+)`)
	for name, src := range progs {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		app := filepath.Join(dir, "app")
		var out, errOut bytes.Buffer
		if code := run(t, []string{"build", "--backend=go", "-o", app, dir}, &out, &errOut); code != 0 {
			t.Fatalf("%s: build: exit %d\n%s%s", name, code, out.String(), errOut.String())
		}
		info, err := exec.Command("go", "version", "-m", app).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: go version -m: %v\n%s", name, err, info)
		}
		m := re.FindSubmatch(info)
		if m == nil {
			t.Fatalf("%s: no -tags in the build information:\n%s", name, info)
		}
		have := map[string]bool{}
		for _, x := range strings.Split(string(m[1]), ",") {
			have[x] = true
		}
		for _, x := range want[name][0] {
			if !have[x] {
				t.Errorf("%s: built without %s (tags %s)", name, x, m[1])
			}
		}
		for _, x := range want[name][1] {
			if have[x] {
				t.Errorf("%s: built with %s, but it reaches the module (tags %s)", name, x, m[1])
			}
		}
		got, err := exec.Command(app).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: run: %v\n%s", name, err, got)
		}
	}
}
