package build

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/gen"
)

const stampSrc = `use std/cli

const VERSION: Str = "dev"
const COMMIT: Str = "unknown"
const BANNER: Str = "tool " + VERSION
const COUNT = 3

fn main(os: Os) {
  let p = cli.parse_or_exit(cli.command("tool", "A tool.", version: VERSION), os.env, os.stdio, os.proc)
  os.stdio.println("${BANNER} (${COMMIT}) ${p.command()}")
}
`

// TestStamp builds a program with `--stamp` (decision 0165) on the Go
// backend and a native one: the constants read the stamped text, and
// constants built from them follow; without stamps the declared values
// stay.
func TestStamp(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	src := filepath.Join(t.TempDir(), "tool")
	os.MkdirAll(src, 0o755)
	os.WriteFile(filepath.Join(src, "main.xo"), []byte(stampSrc), 0o644)
	backends := []string{BackendGo}
	if _, err := Clang(); err == nil {
		backends = append(backends, NativeBackend())
	}
	for _, be := range backends {
		t.Run(be, func(t *testing.T) {
			t.Setenv("XO_BUILD_DIR", t.TempDir())
			run := func(stamps map[string]string, args ...string) string {
				t.Helper()
				res, err := Build([]string{src}, Options{Mode: gen.Build, Backend: be, NoGC: be != BackendGo, Stamps: stamps})
				if err != nil || res.Failed {
					t.Fatalf("build: %v %+v", err, res)
				}
				out, err := exec.Command(res.Binaries[Host()], args...).Output()
				if err != nil {
					t.Fatalf("run: %v", err)
				}
				return string(out)
			}
			if got := run(map[string]string{"VERSION": "1.4.0", "COMMIT": "abc123"}); got != "tool 1.4.0 (abc123) \n" {
				t.Errorf("stamped: %q", got)
			}
			if got := run(map[string]string{"VERSION": "1.4.0"}, "--version"); got != "tool 1.4.0\n" {
				t.Errorf("--version: %q", got)
			}
			if got := run(nil); got != "tool dev (unknown) \n" {
				t.Errorf("not stamped: %q", got)
			}
		})
	}
}

// TestStampErrors: a stamp naming no constant, or a constant that is not
// a plain Str literal, is XO1501 and nothing is built.
func TestStampErrors(t *testing.T) {
	src := filepath.Join(t.TempDir(), "tool")
	os.MkdirAll(src, 0o755)
	os.WriteFile(filepath.Join(src, "main.xo"), []byte(stampSrc), 0o644)
	for name, want := range map[string]string{
		"MISSING": "the main module has no constant `MISSING`",
		"COUNT":   "`COUNT` must be declared `const COUNT: Str = \"...\"`",
		"BANNER":  "`BANNER` must be declared `const BANNER: Str = \"...\"`",
	} {
		res, err := Build([]string{src}, Options{Mode: gen.Build, Stamps: map[string]string{name: "x"}})
		if err != nil {
			t.Fatal(err)
		}
		if !res.Failed {
			t.Errorf("%s: built", name)
			continue
		}
		var msgs []string
		for _, d := range res.Diagnostics {
			if d.Code == string(diag.StampTarget) {
				msgs = append(msgs, d.Message)
			}
		}
		if len(msgs) != 1 || !strings.Contains(msgs[0], want) {
			t.Errorf("%s: diagnostics %q, want one XO1501 with %q", name, msgs, want)
		}
	}
	if _, _, err := ParseStamp("NOEQUALS"); err == nil {
		t.Error("ParseStamp accepted NOEQUALS")
	}
	if n, v, err := ParseStamp("A=b=c"); err != nil || n != "A" || v != "b=c" {
		t.Errorf("ParseStamp(A=b=c) = %q %q %v", n, v, err)
	}
}
