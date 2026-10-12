package llvm_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/llvm"
)

// TestSplitLinks compiles the modules of splitIR separately, links them,
// and runs the program.
func TestSplitLinks(t *testing.T) {
	if _, err := build.Clang(); err != nil {
		t.Skip("clang not found")
	}
	dir := t.TempDir()
	for _, n := range []int{1, 2, 3, 4} {
		var lls []string
		for i, m := range llvm.Split([]byte(llvm.SplitTestIR), n) {
			ll := filepath.Join(dir, "m"+string(rune('0'+n))+string(rune('a'+i))+".ll")
			if err := os.WriteFile(ll, m, 0o644); err != nil {
				t.Fatal(err)
			}
			lls = append(lls, ll)
		}
		// clang compiles each .ll file as its own module
		bin := filepath.Join(dir, "prog"+string(rune('0'+n)))
		if err := build.CompileC([]string{"-O2", "-Wno-override-module"}, lls, bin); err != nil {
			t.Fatalf("n=%d: %v", n, err)
		}
		out, err := exec.Command(bin).Output()
		if err != nil {
			t.Fatalf("n=%d: run: %v", n, err)
		}
		// sum: twice(20) + 1 + eq(20,20)=1 -> 42; twice -> 84
		if string(out) != "total @84\n" {
			t.Errorf("n=%d: got %q", n, out)
		}
	}
}
