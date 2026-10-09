package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildCI checks `xo build --ci` (core 10.5, 11.4): an unformatted
// file and a `dbg(...)` left in code each fail the build before anything
// is compiled; a clean program builds.
func TestBuildCI(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"unformatted", "fn main(os: Os) {\n    os.stdio.println(\"hi\")\n}\n", "not formatted"},
		{"dbg", "fn main(os: Os) {\n  let x = dbg(1 + 2)\n  os.stdio.println(\"${x}\")\n}\n", "error XO0901"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := writeFiles(t, map[string]string{"app/main.xo": c.src})
			app := filepath.Join(dir, "app")
			var out, errOut bytes.Buffer
			if code := run([]string{"build", "--ci", "-o", filepath.Join(dir, "bin"), app}, &out, &errOut); code != 1 {
				t.Fatalf("exit %d, want 1\n%s", code, errOut.String())
			}
			if !strings.Contains(errOut.String(), c.want) || !strings.Contains(errOut.String(), "nothing built") {
				t.Fatalf("stderr does not report %q:\n%s", c.want, errOut.String())
			}
			if _, err := os.Stat(filepath.Join(dir, "bin")); err == nil {
				t.Fatal("a binary was built")
			}
			out.Reset()
			errOut.Reset()
			if code := run([]string{"build", "--ci", "--json", app}, &out, &errOut); code != 1 || !strings.Contains(out.String(), `"ok": false`) {
				t.Fatalf("--json: exit %d\n%s", code, out.String())
			}
		})
	}
	if testing.Short() {
		return
	}
	dir := writeFiles(t, map[string]string{"app/main.xo": "fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n"})
	t.Setenv("XO_BUILD_DIR", filepath.Join(dir, "build"))
	var out, errOut bytes.Buffer
	if code := run([]string{"build", "--ci", "-o", filepath.Join(dir, "bin"), filepath.Join(dir, "app")}, &out, &errOut); code != 0 {
		t.Fatalf("clean program: exit %d\n%s", code, errOut.String())
	}
}
