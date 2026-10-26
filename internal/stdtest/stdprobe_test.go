// A `std/NAME` module's test file is type checked (2026-10-25).
package stdtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestStdModuleTestBodies runs `xo test --std-module=probe` over a std
// whose probe module is a plain `std/NAME` module, the shape of the
// prelude and of every std module whose implementation is a native
// module. The checker skipped the bodies of every std module (a build
// trusts the full std check, compiler/check/dump.xo run), the std module
// under test included, so the test file's bodies were built with no names
// resolved: each list literal and each call on a list in them was XO1201
// "not available on the native backends (arm64, amd64, LLVM) yet ...
// (support is planned)", though the same code compiled in a program. A
// test file beside a native module (std/native/NAME, --std-module=probe
// there) was unaffected, since that module is not a std module.
func TestStdModuleTestBodies(t *testing.T) {
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-stdprobe-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	std := filepath.Join(work, "std")
	if err := os.CopyFS(std, os.DirFS(filepath.Join(root, "std"))); err != nil {
		t.Fatal(err)
	}
	// the module: one function of its own, so the test file has a private
	// name to call as well as the prelude's
	mod := "/// A std module with no native implementation, the shape the\n" +
		"/// std module under test may have (probe_test.xo is its test file).\n" +
		"fn seven() -> Int {\n" +
		"  7\n" +
		"}\n"
	if err := os.WriteFile(filepath.Join(std, "probe.xo"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := "test \"a list literal\" {\n" +
		"  var xs = [3, 1]\n" +
		"  xs.push(4)\n" +
		"  expect(xs.len() == 3)\n" +
		"}\n" +
		"\n" +
		"test \"a prelude constructor and a private function\" {\n" +
		"  let o: Option[Int] = Some(1)\n" +
		"  expect(o.is_some())\n" +
		"  expect(seven() == 7)\n" +
		"}\n"
	if err := os.WriteFile(filepath.Join(std, "probe_test.xo"), []byte(tests), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := testtmp.Guard(exec.Command(xo, "test", "--std-module=probe", "--backend="+backends()[0]))
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "XO_STD="+std, "PWD="+root)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("xo test --std-module=probe: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "2 passed") {
		t.Fatalf("xo test --std-module=probe: no \"2 passed\" in\n%s", out)
	}
}
