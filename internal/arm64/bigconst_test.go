package arm64

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// bigConstSrc returns a program with n distinct string literals of size
// bytes each, about n*size bytes of constant data in one section, and
// the line it prints. Mach-O's ARM64_RELOC_ADDEND holds 24 bits, so a
// reference more than 8 MiB past the section symbol it named failed to
// assemble ("addend 8388672 does not fit ARM64_RELOC_ADDEND"); a large
// generated table in an SDK hits it (gotchas/macho-arm64-addend-8mib.md).
func bigConstSrc(n, size int) (string, string) {
	const digits = "abcdefghijklmnopqrstuvwxyz0123456789"
	var b strings.Builder
	b.WriteString("fn table() -> List[Str] {\n  [\n")
	x := uint64(12345)
	h := 0
	for i := 0; i < n; i++ {
		b.WriteString("    \"")
		var last byte
		for j := 0; j < size; j++ {
			x = x*6364136223846793005 + 1442695040888963407
			last = digits[(x>>33)%uint64(len(digits))]
			b.WriteByte(last)
		}
		b.WriteString("\",\n")
		h = (h*31 + size + int(last)) % 1000000007
	}
	b.WriteString(`  ]
}

fn main(os: Os) {
  let t = table()
  var h = 0
  for s in t {
    h = (h * 31 + s.len() + Int(s.bytes()[s.len() - 1])) % 1000000007
  }
  os.stdio.println("${t.len()} ${t[t.len() - 1].len()} ${h}")
}
`)
	return b.String(), fmt.Sprintf("%d %d %d\n", n, size, h)
}

// TestConstDataPast8MiB builds and runs a program with 11 MB of string
// constants: the last ones lie past ARM64_RELOC_ADDEND's 8 MiB on
// darwin, and past ADRP's 1 MiB in-place addend on windows/arm64. The
// program is built whole, with a back-end cache of its own.
func TestConstDataPast8MiB(t *testing.T) {
	t.Parallel()
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds a program with 11 MB of constants")
	}
	dir, err := testtmp.Dir("xo-bigconst-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	src, want := bigConstSrc(2200, 5000)
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module bigconst\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	// a cache of its own: with the shared cache's separately compiled
	// std the build took another path and passed without the fix
	res, err := buildRunWith(t, dir, xotool.Options{Backend: xotool.BackendArm64, Env: []string{"XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR=" + filepath.Join(dir, "cache")}})
	if err != nil {
		t.Fatal(err)
	}
	if res.exit != 0 || res.stdout != want {
		t.Errorf("exit %d, stdout %q, want %q\nstderr:\n%s", res.exit, res.stdout, want, res.stderr)
	}
}
