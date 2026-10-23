package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildCI checks `xo build --ci` (core 10.5, 11.4): an unformatted
// file and a `dbg(...)` left in code each fail the build before anything
// is compiled; a clean program builds.
func TestBuildCI(t *testing.T) {
	t.Parallel()
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
			if code := run(t, []string{"build", "--ci", "-o", filepath.Join(dir, "bin"), app}, &out, &errOut); code != 1 {
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
			if code := run(t, []string{"build", "--ci", "--json", app}, &out, &errOut); code != 1 || !strings.Contains(out.String(), `"ok": false`) {
				t.Fatalf("--json: exit %d\n%s", code, out.String())
			}
		})
	}
	if testing.Short() {
		return
	}
	dir := writeFiles(t, map[string]string{"app/main.xo": "fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n"})
	setenv(t, "XO_BUILD_DIR", filepath.Join(dir, "build"))
	var out, errOut bytes.Buffer
	if code := run(t, []string{"build", "--ci", "-o", filepath.Join(dir, "bin"), filepath.Join(dir, "app")}, &out, &errOut); code != 0 {
		t.Fatalf("clean program: exit %d\n%s", code, errOut.String())
	}
}

// TestBuildRelease checks `xo build --release` (core 8): a contract
// marked `requires(debug)` is compiled out, while `--ci` alone keeps it
// (--ci is a gate, not a build mode).
func TestBuildRelease(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds and runs programs")
	}
	src := "fn half(n: Int) -> Int\n  requires(debug) n % 2 == 0\n{\n  n / 2\n}\n\nfn main(os: Os) {\n  os.stdio.println(\"${half(7)}\")\n}\n"
	dir := writeFiles(t, map[string]string{"app/main.xo": src})
	setenv(t, "XO_BUILD_DIR", filepath.Join(dir, "build"))
	setenv(t, "XO_CRASH_DIR", dir)
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
		if code := run(t, []string{"build", c.flag, "-o", bin, app}, &out, &errOut); code != 0 {
			t.Fatalf("%s: exit %d\n%s", c.flag, code, errOut.String())
		}
		cmd := command(t, bin)
		got, _ := cmd.Output()
		if string(got) != c.out || cmd.ProcessState.ExitCode() != c.exit {
			t.Errorf("%s: printed %q, exit %d; want %q, exit %d", c.flag, got, cmd.ProcessState.ExitCode(), c.out, c.exit)
		}
	}
}

// TestBuildOutPaths covers where a build writes (2026-10-22, the
// owner's 0.25.2 reports): -o naming an existing directory writes
// DIR/NAME (and NAME.dSYM) inside it, as go build -o does; a failed
// write leaves no OUT.tmp and says why in words; xo run leaves nothing
// in the project.
func TestBuildOutPaths(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{
		"webserver/main.xo": "fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n",
		"out/.keep":         "",
		"busy/webserver/x":  "a directory where the binary would go\n",
	})
	app := filepath.Join(dir, "webserver")
	chdir(t, app)
	var out, errOut bytes.Buffer
	if code := run(t, []string{"build", "-o", filepath.Join(dir, "out"), "."}, &out, &errOut); code != 0 {
		t.Fatalf("-o DIR: exit %d\n%s", code, errOut.String())
	}
	if fi, err := os.Stat(filepath.Join(dir, "out", "webserver")); err != nil || fi.IsDir() {
		t.Errorf("-o DIR did not write DIR/webserver: %v", err)
	}
	errOut.Reset()
	if code := run(t, []string{"build", "-o", filepath.Join(dir, "busy"), "."}, &out, &errOut); code == 0 {
		t.Fatalf("-o DIR with DIR/webserver a directory: exit 0")
	}
	if strings.Contains(errOut.String(), "IsDir(") || !strings.Contains(errOut.String(), "cannot write the program") {
		t.Errorf("want a readable error:\n%s", errOut.String())
	}
	ents, _ := os.ReadDir(filepath.Join(dir, "busy"))
	for _, e := range ents {
		if e.Name() != "webserver" {
			t.Errorf("a failed build left %s", e.Name())
		}
	}
	errOut.Reset()
	if code := run(t, []string{"run", "."}, &out, &errOut); code != 0 {
		t.Fatalf("run: exit %d\n%s", code, errOut.String())
	}
	ents, _ = os.ReadDir(app)
	for _, e := range ents {
		if e.Name() != "main.xo" {
			t.Errorf("xo run left %s in the project", e.Name())
		}
	}
}

// TestBuildSettings checks decision 0354: --release is a preset, and each
// setting has its own flag in any build that wins over the preset
// whatever the order; the flags' usage errors.
func TestBuildSettings(t *testing.T) {
	t.Parallel()
	src := `enum InputErr {
  Odd(n: Int)
} derive Display

fn checked_half(n: Int) -> Int ! InputErr
  requires(debug) n % 2 == 0 else InputErr.Odd(n: n)
{
  n / 2
}

fn load(id: Int) -> Int ! InputErr {
  if id > 10 {
    return Err(InputErr.Odd(n: id))
  }
  id
}

fn profile(id: Int, name: Str) -> Str ! InputErr {
  let h = load(id)?
  "${name}: ${h}"
}

fn main(os: Os) ! {
  os.stdio.println("contracts ${!checked_half(7).is_ok()}")
  let x = dbg(1 + 2)
  os.stdio.println("x ${x}")
  os.stdio.println(profile(42, "b")?)
}
`
	dir := writeFiles(t, map[string]string{"app/main.xo": src})
	app := filepath.Join(dir, "app")
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--debug-info=split"}, "decision 0355"},
		{[]string{"--backend=arm64", "--opt=O2"}, "has no optimization levels"},
		{[]string{"--opt=O9"}, "--opt"},
		{[]string{"--dbg=maybe"}, "--dbg"},
	} {
		var o, e bytes.Buffer
		args := append(append([]string{"build"}, c.args...), app)
		if code := run(t, args, &o, &e); code != 2 || !strings.Contains(e.String(), c.want) {
			t.Errorf("xo %s: exit %d, want 2 and %q:\n%s", strings.Join(args, " "), code, c.want, e.String())
		}
	}
	if testing.Short() {
		t.Skip("builds and runs programs")
	}
	setenv(t, "XO_BUILD_DIR", filepath.Join(dir, "build"))
	setenv(t, "XO_CRASH_DIR", dir)
	for i, c := range []struct {
		flags                []string
		contracts, dbg, args bool
	}{
		{nil, true, true, true},
		{[]string{"--release"}, false, false, true},
		{[]string{"--release", "--debug-contracts=on"}, true, false, true},
		{[]string{"--debug-contracts=on", "--release"}, true, false, true},
		{[]string{"--debug-contracts=off"}, false, true, true},
		{[]string{"--dbg=off"}, true, false, true},
		{[]string{"--release", "--dbg"}, false, true, true},
		{[]string{"--error-args=off"}, true, true, false},
		{[]string{"--release", "--error-args=false"}, false, false, false},
	} {
		name := strings.Join(c.flags, " ")
		bin := filepath.Join(dir, fmt.Sprintf("bin%d", i))
		var out, errOut bytes.Buffer
		args := append(append([]string{"build"}, c.flags...), "-o", bin, app)
		if code := run(t, args, &out, &errOut); code != 0 {
			t.Errorf("xo %s: exit %d\n%s", strings.Join(args, " "), code, errOut.String())
			continue
		}
		cmd := command(t, bin)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		got, _ := cmd.Output()
		if want := fmt.Sprintf("contracts %v\nx 3\n", c.contracts); !strings.HasPrefix(string(got), want) {
			t.Errorf("[%s] stdout %q, want prefix %q", name, got, want)
		}
		if has := strings.Contains(stderr.String(), "1 + 2 = 3"); has != c.dbg {
			t.Errorf("[%s] dbg printed %v, want %v:\n%s", name, has, c.dbg, stderr.String())
		}
		if has := strings.Contains(stderr.String(), "profile(id=42"); has != c.args {
			t.Errorf("[%s] frame with arguments %v, want %v:\n%s", name, has, c.args, stderr.String())
		}
		if !c.args && !strings.Contains(stderr.String(), "profile(...)") {
			t.Errorf("[%s] no profile(...) frame:\n%s", name, stderr.String())
		}
	}
}
