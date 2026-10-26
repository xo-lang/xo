// Shared plumbing for the private-module fuzz probes: a std copy in a
// temp dir with a generated test file beside one of its modules, run with
// `xo test --std-module=MODULE` (the shape of stdprobe_test.go, so a test
// can reach a std module's private functions).
package stdtest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// probeModule runs content as MODULE's _test.xo file in a temp copy of std
// and fails the test with the run's output when xo test fails. module is
// the --std-module value (native/llrt/http1); file its relative path
// under std. backends are xo backend names (arm64, llvm). cases is how
// many tests content holds: xo test's totals must match it, so a
// generated file that is not picked up (a wrong name, a module layout
// change) or a name collision that drops cases fails here instead of
// passing a comparison of nothing.
func probeModule(t *testing.T, module, file, content string, backends []string, cases int) {
	t.Helper()
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-probe-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	std := filepath.Join(work, "std")
	if err := os.CopyFS(std, os.DirFS(filepath.Join(root, "std"))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(std, file), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, b := range backends {
		cmd := testtmp.Guard(exec.Command(xo, "test", "--std-module="+module, "--backend="+b))
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "XO_STD="+std, "PWD="+root)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("xo test --std-module=%s --backend=%s: %v\n%s", module, b, err, out)
		}
		passed, failed, ok := probeSummary(string(out))
		if !ok {
			t.Fatalf("xo test --std-module=%s --backend=%s: no \"N passed\" summary in\n%s", module, b, out)
		}
		t.Logf("%s/%s: %d passed, %d failed of %d generated cases", module, b, passed, failed, cases)
		if failed != 0 || passed != cases {
			t.Fatalf("xo test --std-module=%s --backend=%s ran %d passed, %d failed; the generated file holds %d tests (a file that is not picked up would otherwise compare nothing)\n%s",
				module, b, passed, failed, cases, out)
		}
	}
}

// probeSummaryRE matches xo test's last line: "ok: 2 passed, 0 failed, 0
// skipped" ("FAIL: ..." when something failed; compiler/xo/test.xo).
var probeSummaryRE = regexp.MustCompile(`(?:ok|FAIL): (\d+) passed, (\d+) failed, (\d+) skipped`)

// probeSummary reads xo test's totals.
func probeSummary(out string) (passed, failed int, ok bool) {
	m := probeSummaryRE.FindStringSubmatch(out)
	if m == nil {
		return 0, 0, false
	}
	passed, _ = strconv.Atoi(m[1])
	failed, _ = strconv.Atoi(m[2])
	return passed, failed, true
}

// envInt reads an int knob: def when the variable is unset or empty.
func envInt(t *testing.T, name string, def int) int {
	t.Helper()
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return n
}

// envBackends reads a comma separated backend list: def when unset.
func envBackends(name string, def []string) []string {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	return strings.Split(v, ",")
}

// xoStr is s as an Xo string literal.
func xoStr(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '$':
			b.WriteString(`\$`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u{%x}`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
