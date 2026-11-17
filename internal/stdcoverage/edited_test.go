package stdcoverage

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

// TestModuleOfStdFile: the API module each std file belongs to (task #259):
// a module's declaration and implementation files are it, the prelude's is
// prelude, and llrt, std/rt, tests and docs declare no API.
func TestModuleOfStdFile(t *testing.T) {
	for _, c := range []struct{ path, want string }{
		{"std/toml.xo", "toml"},
		{"std/native/toml.xo", "toml"},
		{"std/native/adler32_native.xo", "adler32"},
		{"std/native/prelude.xo", "prelude"},
		{"std/toml_test.xo", ""},
		{"std/native/llrt/scan.xo", ""},
		{"std/rt/toml.go", ""},
		{"std/changes/toml.md", ""},
		{"compiler/xo/main.xo", ""},
	} {
		if got := moduleOfStdFile(c.path); got != c.want {
			t.Errorf("moduleOfStdFile(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

// TestEditedModules: the modules a change edits, and the modules a
// program the change edits or deletes used at the base (they may have lost
// their last caller), from git.
func TestEditedModules(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("needs git")
	}
	root := t.TempDir()
	write := func(p, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, p), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run := func(args ...string) {
		t.Helper()
		c := exec.Command("git", append([]string{"-c", "user.email=t@example.com", "-c", "user.name=t"}, args...)...)
		c.Dir = root
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write("std/toml.xo", "fn decode() {}\n")
	write("std/yaml.xo", "fn decode() {}\n")
	write("std/native/llrt/scan.xo", "fn f() {}\n")
	write("internal/llvm/testdata/golden/uses_yaml/main.xo", "use std/yaml\n\nfn main(os: Os) {}\n")
	write("internal/llvm/testdata/golden/gone/main.xo", "use std/json\nuse std/csv\n\nfn main(os: Os) {}\n")
	run("init", "-q")
	run("add", "-A")
	run("commit", "-qm", "base")
	write("std/native/toml.xo", "fn decode() {}\n")
	write("std/native/llrt/scan.xo", "fn f() { }\n")
	write("internal/llvm/testdata/golden/uses_yaml/main.xo", "fn main(os: Os) {}\n")
	if err := os.RemoveAll(filepath.Join(root, "internal/llvm/testdata/golden/gone")); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	progs := []string{"internal/llvm/testdata/golden/uses_yaml", "internal/llvm/testdata/golden/gone"}
	edited, lost, err := EditedModules(root, "HEAD", progs)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"toml"}; !reflect.DeepEqual(edited, want) {
		t.Errorf("edited = %v, want %v", edited, want)
	}
	if want := []string{"csv", "json", "yaml"}; !reflect.DeepEqual(lost, want) {
		t.Errorf("lost callers = %v, want %v", lost, want)
	}
}
