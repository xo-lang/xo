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

// TestLayoutStoreOrder (decision 0403 stage 1b): a separately compiled
// program's link places the text atoms a layout file names first, in its
// order: the program's hot function, then the std functions named, before
// the store's other atoms (nm -n, darwin).
func TestLayoutStoreOrder(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	if !native() || os.Getenv("XO_HOST_OS") == "linux" {
		t.Skip("a darwin store link")
	}
	if _, err := exec.LookPath("nm"); err != nil {
		t.Skip("no nm")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-layoutstore-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	cache := filepath.Join(work, "cache")
	cmd := testtmp.Guard(exec.Command(xo, "dev", "stdcache", "--conc=0"))
	cmd.Env = append(os.Environ(), "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+cache)
	paths, err := cmd.Output()
	if err != nil {
		t.Fatalf("xo dev stdcache: %v", err)
	}
	lines := strings.Fields(string(paths))
	if len(lines) != 2 {
		t.Fatalf("xo dev stdcache printed %q, want the store and the facts", paths)
	}
	store, facts := lines[0], lines[1]
	prog := filepath.Join(work, "prog")
	if err := os.MkdirAll(prog, 0o755); err != nil {
		t.Fatal(err)
	}
	// hot is recursive, so it is not inlined into main and keeps its symbol
	src := `fn hot(s: Str, d: Int) -> Int {
  if d == 0 {
    return s.split(",").len()
  }
  hot(s, d - 1) + s.trim_suffix("x").len()
}

fn main(os: Os) {
  os.stdio.println("${hot("ax,bx,cx", 3)}")
}
`
	if err := os.WriteFile(filepath.Join(prog, "main.xo"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prog, "xo.mod"), []byte("module prog\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// hot first, then two std functions the program reaches
	layout := "xo layout v1\ntarget darwin/arm64\nsamples 10 hz 100\nfn hot 8\nfn prelude.str_trim_suffix 1\nfn prelude.str_split 1\nedge hot prelude.str_trim_suffix 1\n"
	lf := filepath.Join(work, "xo.layout")
	if err := os.WriteFile(lf, []byte(layout), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(work, "prog.bin")
	b := testtmp.Guard(exec.Command(xo, "build", "--layout="+lf, "-o", out, "."))
	b.Dir = prog
	b.Env = append(os.Environ(), "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+cache, "XO_CACHE_STATS=1",
		"XO_SEPCOMP=1", "XO_SEPCOMP_FACTS="+facts, "XO_SEPCOMP_STORE="+store)
	msgs, err := b.CombinedOutput()
	if err != nil {
		t.Fatalf("build: %v\n%s", err, msgs)
	}
	if !strings.Contains(string(msgs), "link: layout order of 3 symbols") {
		t.Fatalf("the link did not get the layout's order:\n%s", msgs)
	}
	nm, err := exec.Command("nm", "-n", out).Output()
	if err != nil {
		t.Fatal(err)
	}
	var text []string
	for _, l := range strings.Split(string(nm), "\n") {
		f := strings.Fields(l)
		if len(f) == 3 && (f[1] == "T" || f[1] == "t") && strings.HasPrefix(f[2], "_xo_f_") {
			text = append(text, f[2])
		}
	}
	want := []string{"_xo_f_hot", "_xo_f_prelude_2estr__trim__suffix", "_xo_f_prelude_2estr__split"}
	if len(text) < 3 {
		t.Fatalf("too few text symbols: %v", text)
	}
	for i, w := range want {
		if text[i] != w {
			t.Errorf("text symbol %d is %s, want %s (first symbols: %v)", i, text[i], w, text[:6])
		}
	}
}
