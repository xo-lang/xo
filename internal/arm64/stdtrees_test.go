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

// TestStdTreesRebuild exercises decision 0388 stage 2's fallback: a
// separately compiled build whose std records leave every tree pending
// (XO_STD_TREES_TEST=1) meets a std body without its declaration in the
// SSA builder, joins every tree and builds the front end again
// (phase std_trees_rebuild), and the program's output is the same as the
// build that parsed std's generic files before install.
func TestStdTreesRebuild(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-stdtrees-")
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
	// a generic std function instantiated with the program's own type:
	// not among std's precompiled instances, so the SSA builder lowers
	// its body from the tree
	src := `struct Rec {
  key: Int
  name: Str
}

fn main(os: Os) {
  var rs: List[Rec] = []
  for i in 0..20 {
    rs.push(Rec{key: (i * 7) % 13, name: "r${i}"})
  }
  let sorted = rs.sort_by(fn(r) { r.key })
  let names = sorted.map(fn(r) { r.name })
  os.stdio.println("${sorted[0].key} ${sorted[19].key} ${names[0]} edit 0")
}
`
	prog := filepath.Join(work, "prog")
	if err := os.MkdirAll(prog, 0o755); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+cache, "XO_STD_EXPORT=1",
		"XO_SEPCOMP=1", "XO_SEPCOMP_FACTS="+facts, "XO_SEPCOMP_STORE="+store)
	build := func(edit int, extra ...string) (string, string) {
		if err := os.WriteFile(filepath.Join(prog, "main.xo"), []byte(strings.Replace(src, "edit 0", "edit "+string(rune('0'+edit)), 1)), 0o644); err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(work, "prog"+string(rune('0'+edit)))
		b := testtmp.Guard(exec.Command(xo, "build", "--phase-times", "-o", out, "."))
		b.Dir = prog
		b.Env = append(append([]string{}, env...), extra...)
		msgs, err := b.CombinedOutput()
		if err != nil {
			t.Fatalf("build %d: %v\n%s", edit, err, msgs)
		}
		run := testtmp.Guard(exec.Command(out))
		got, err := run.Output()
		if err != nil {
			t.Fatalf("run %d: %v", edit, err)
		}
		return string(msgs), string(got)
	}
	// the declaring build, then one from the record with std's generic
	// files parsed before install (the usual path)
	build(0)
	msgs1, out1 := build(1)
	if strings.Contains(msgs1, "std_trees_rebuild") {
		t.Fatalf("the usual record path rebuilt:\n%s", msgs1)
	}
	// every tree pending: the SSA builder meets one, the front end is
	// built again, the output is the same
	msgs2, out2 := build(2, "XO_STD_TREES_TEST=1")
	if !strings.Contains(msgs2, "phase std_trees_rebuild") {
		t.Fatalf("no std_trees_rebuild phase with every tree pending:\n%s", msgs2)
	}
	want1 := strings.Replace(out1, "edit 1", "edit 2", 1)
	if out2 != want1 {
		t.Fatalf("outputs differ: %q against %q", out2, want1)
	}
}
