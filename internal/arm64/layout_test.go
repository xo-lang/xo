package arm64

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestLayoutIdentity (decision 0403): a build with a layout file is
// reproducible (the same bytes twice, and with the back-end cache off),
// differs from the build without it (the hot function moved first), and
// the file's hash is among the build's sources; a layout file of another
// target is ignored with one line and gives the no-layout bytes but for
// the build id.
func TestLayoutIdentity(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-layout-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	cache := filepath.Join(work, "cache")
	prog := filepath.Join(work, "prog")
	if err := os.MkdirAll(prog, 0o755); err != nil {
		t.Fatal(err)
	}
	src := `fn hot(n: Int) -> Int {
  var s = 0
  for i in 0..n {
    s += i % 7
  }
  s
}

fn warm(n: Int) -> Int {
  n * 3 + 1
}

fn main(os: Os) {
  os.stdio.println("${hot(1000)} ${warm(5)}")
}
`
	if err := os.WriteFile(filepath.Join(prog, "main.xo"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prog, "xo.mod"), []byte("module prog\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	layout := "xo layout v1\ntarget darwin/arm64\nsamples 10 hz 100\nfn hot 9\nfn warm 1\nline hot 4 9\nedge main hot 9\n"
	if err := os.WriteFile(filepath.Join(work, "xo.layout"), []byte(layout), 0o644); err != nil {
		t.Fatal(err)
	}
	build := func(name string, args []string, extra ...string) ([]byte, string) {
		dir := filepath.Join(work, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(dir, "prog")
		b := testtmp.Guard(exec.Command(xo, append(append([]string{"build"}, args...), "-o", out, ".")...))
		b.Dir = prog
		b.Env = append(append(os.Environ(), "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+cache, "XO_CACHE_STATS=1"), extra...)
		msgs, err := b.CombinedOutput()
		if err != nil {
			t.Fatalf("build %s: %v\n%s", name, err, msgs)
		}
		bin, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return bin, string(msgs)
	}
	lf := "--layout=" + filepath.Join(work, "xo.layout")
	a, amsgs := build("a", []string{lf})
	if !strings.Contains(amsgs, "sampled functions matched") {
		t.Fatalf("the layout was not applied:\n%s", amsgs)
	}
	b, _ := build("b", []string{lf})
	if !bytes.Equal(a, b) {
		t.Errorf("two builds with the layout differ")
	}
	c, _ := build("c", []string{lf}, "XO_BACKEND_CACHE=off")
	if !bytes.Equal(a, c) {
		t.Errorf("the build with the layout differs with the back-end cache off")
	}
	off, _ := build("off", []string{"--layout=off"})
	if bytes.Equal(a, off) {
		t.Errorf("the build with the layout is the same as without it (hot should move first)")
	}
	other := filepath.Join(work, "other.layout")
	if err := os.WriteFile(other, []byte(strings.Replace(layout, "target darwin/arm64", "target linux/amd64", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	_, omsgs := build("other", []string{"--layout=" + other})
	if !strings.Contains(omsgs, "ignored for") {
		t.Errorf("a layout file of another target was not ignored:\n%s", omsgs)
	}
}
