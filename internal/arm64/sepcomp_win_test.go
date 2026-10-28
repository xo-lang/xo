package arm64

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestSepcompLinksWindows (decision 0388 stage 3 on windows/amd64): std's
// windows store and facts from the std cache (xo dev stdcache
// --target=windows/amd64), and a program built against them (XO_SEPCOMP:
// its objects whole, std through pelink's COFF store link) links with its
// PDB, before and after an edit. The program instantiates a generic std
// type std also has (a weak definition each side, the store's kept). The
// executables run under Wine in the gate, not here.
func TestSepcompLinksWindows(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-sepcomp-win-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	cmd := testtmp.Guard(exec.Command(xo, "dev", "stdcache", "--target=windows/amd64", "--conc=0"))
	cmd.Env = append(os.Environ(), "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "c-std"))
	paths, err := cmd.Output()
	if err != nil {
		t.Fatalf("xo dev stdcache: %v", err)
	}
	lines := strings.Fields(string(paths))
	if len(lines) != 2 {
		t.Fatalf("xo dev stdcache printed %q, want the store and the facts", paths)
	}
	store, facts := lines[0], lines[1]
	src := `fn main(os: Os) {
  var m: Map[Str, Int] = {}
  for w in "edit 0 of the program".split(" ") {
    m[w] = w.len()
  }
  var xs: List[Int] = []
  for i in 0..20 {
    xs.push((i * 7) % 13)
  }
  os.stdio.println("${xs.sort().len()} ${m.len()} ${m.get("program") ?? 0}")
}
`
	prog := filepath.Join(work, "prog")
	if err := os.MkdirAll(prog, 0o755); err != nil {
		t.Fatal(err)
	}
	for edit := 0; edit < 2; edit++ {
		if err := os.WriteFile(filepath.Join(prog, "main.xo"), []byte(strings.Replace(src, "edit 0", "edit "+string(rune('0'+edit)), 1)), 0o644); err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(work, "sep", "prog")
		os.Remove(out + ".exe")
		os.Remove(out + ".pdb")
		b := testtmp.Guard(exec.Command(xo, "build", "--backend=amd64", "--targets=windows/amd64", "-o", out, "."))
		b.Dir = prog
		b.Env = append(os.Environ(), "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "c-sep"), "XO_SEPCOMP=1", "XO_SEPCOMP_FACTS="+facts, "XO_SEPCOMP_STORE="+store)
		if log, err := b.CombinedOutput(); err != nil {
			t.Fatalf("edit %d: xo build (XO_SEPCOMP, windows/amd64): %v\n%s", edit, err, log)
		}
		for _, f := range []string{out + ".exe", out + ".pdb"} {
			if fi, err := os.Stat(f); err != nil || fi.Size() == 0 {
				t.Fatalf("edit %d: %s missing or empty (%v)", edit, f, err)
			}
		}
	}
}
