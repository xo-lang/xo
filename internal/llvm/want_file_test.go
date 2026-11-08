package llvm

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWantFileFor: a golden's want_<goos>_<goarch>.txt wins over its
// want_<goos>.txt, which wins over want.txt (f32_bits keeps riscv64's
// canonical NaNs in want_linux_riscv64.txt, decision 0361).
func TestWantFileFor(t *testing.T) {
	dir := t.TempDir()
	write := func(name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("exit 0\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	check := func(goos, goarch, want string) {
		t.Helper()
		if got := filepath.Base(wantFileFor(dir, goos, goarch)); got != want {
			t.Errorf("wantFileFor(%s/%s) = %s, want %s", goos, goarch, got, want)
		}
	}
	write("want.txt")
	check("linux", "riscv64", "want.txt")
	write("want_linux.txt")
	check("linux", "riscv64", "want_linux.txt")
	check("darwin", "arm64", "want.txt")
	write("want_linux_riscv64.txt")
	check("linux", "riscv64", "want_linux_riscv64.txt")
	check("linux", "arm64", "want_linux.txt")
	check("darwin", "riscv64", "want.txt")
}
