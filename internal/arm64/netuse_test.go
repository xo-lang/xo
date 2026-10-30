package arm64

import (
	"debug/elf"
	"debug/macho"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestNetRuntimeOnUse: the native prelude imports cabi_net (os.net's
// runtime calls lower to its exports), but a program that makes no
// network call builds none of it: the prelude, built in every program,
// does not count as cabi_net's importer (ssa importer_built), and a body
// that lowers a network call sets uses_net (net_rt). Before, llvm-fib
// built 129 functions of the network runtime (about 11 ms of an edit,
// 2026-10-27). A program that listens through os.net links it.
func TestNetRuntimeOnUse(t *testing.T) {
	t.Parallel()
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-netuse-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	syms := func(dir string) map[string]bool {
		t.Helper()
		out := filepath.Join(work, "bin-"+filepath.Base(dir))
		cmd := testtmp.Guard(exec.Command(xo, "build", "--backend=arm64", "-o", out, "."))
		cmd.Dir = dir
		if log, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("xo build %s: %v\n%s", dir, err, log)
		}
		m := map[string]bool{}
		if runtime.GOOS == "linux" {
			// an ELF binary on the linux/arm64 host (2026-10-30)
			f, err := elf.Open(out)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			syms, _ := f.Symbols()
			for _, s := range syms {
				m[s.Name] = true
			}
			return m
		}
		f, err := macho.Open(out)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if f.Symtab != nil {
			for _, s := range f.Symtab.Syms {
				m[s.Name] = true
			}
		}
		return m
	}
	plain := filepath.Join(work, "plain")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plain, "main.xo"), []byte("fn main(os: Os) {\n  os.stdio.println(\"hello\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for n := range syms(plain) {
		if strings.Contains(n, "xo_x_nc_") {
			t.Errorf("a program with no network call has %s", n)
		}
	}
	// Mach-O names carry a leading underscore, ELF names do not (the
	// linux/arm64 host, 2026-10-30)
	listen := "_xo_x_nc_listen"
	if runtime.GOOS == "linux" {
		listen = "xo_x_nc_listen"
	}
	if !syms(filepath.Join("..", "llvm", "testdata", "golden", "conn_iface"))[listen] {
		t.Errorf("conn_iface (os.net.listen) has no %s", listen)
	}
}
