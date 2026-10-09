package main

import (
	"bytes"
	"os"
	"os/exec"
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

// TestBuildRelease checks `xo build --release` (core 8): a contract
// marked `requires(debug)` is compiled out, while `--ci` alone keeps it
// (--ci is a gate, not a build mode).
func TestBuildRelease(t *testing.T) {
	// --error-args=false belongs to release builds (decision 0092)
	var o, e bytes.Buffer
	if code := run([]string{"build", "--error-args=false", t.TempDir()}, &o, &e); code != 2 || !strings.Contains(e.String(), "--error-args=false needs --release") {
		t.Errorf("--error-args=false without --release: exit %d\n%s", code, e.String())
	}
	if testing.Short() {
		t.Skip("builds and runs programs")
	}
	src := "fn half(n: Int) -> Int\n  requires(debug) n % 2 == 0\n{\n  n / 2\n}\n\nfn main(os: Os) {\n  os.stdio.println(\"${half(7)}\")\n}\n"
	dir := writeFiles(t, map[string]string{"app/main.xo": src})
	t.Setenv("XO_BUILD_DIR", filepath.Join(dir, "build"))
	t.Setenv("XO_CRASH_DIR", dir)
	app := filepath.Join(dir, "app")
	for _, c := range []struct {
		flag, out string
		exit      int
	}{
		{"--release", "3\n", 0},
		{"--ci", "", 70},
	} {
		var out, errOut bytes.Buffer
		bin := filepath.Join(dir, "bin"+c.flag)
		if code := run([]string{"build", c.flag, "-o", bin, app}, &out, &errOut); code != 0 {
			t.Fatalf("%s: exit %d\n%s", c.flag, code, errOut.String())
		}
		cmd := exec.Command(bin)
		got, _ := cmd.Output()
		if string(got) != c.out || cmd.ProcessState.ExitCode() != c.exit {
			t.Errorf("%s: printed %q, exit %d; want %q, exit %d", c.flag, got, cmd.ProcessState.ExitCode(), c.out, c.exit)
		}
	}
}
