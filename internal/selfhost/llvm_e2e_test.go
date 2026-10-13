package selfhost

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestSelfhostLLVMEndToEnd is stage 4 of decision 0166: the self hosted
// pipeline (selfhost/check, selfhost/ssa and its passes, selfhost/llvm,
// in the binary of selfhost/llvm built with the go backend, or the one
// XO_SELFHOST_BACKEND names) lowers a self hosted component, its IR must
// be the recorded one (internal/llvm's until decision 0215), and `xo
// build --backend=llvm` (the same pipeline in xo, no binary cache) must
// build a binary whose output on a fixed input is the go backend build's
// output. The component is
// selfhost/lexer, or the one XO_SELFHOST_E2E names (lexer, parser,
// check, ssa, llvm; clang on the IR of ssa took 34
// minutes). Full tier only, unless XO_SELFHOST_E2E is set.
func TestSelfhostLLVMEndToEnd(t *testing.T) {
	comp := os.Getenv("XO_SELFHOST_E2E")
	if comp == "" {
		if selfhostTier(t) != tierFull {
			t.Skip("full tier (XO_SELFHOST_TIER=full) or XO_SELFHOST_E2E=<component>")
		}
		comp = "lexer"
	}
	if why := nativeSkip("llvm"); why != "" {
		t.Skip(why)
	}
	// what the built binary is run on, from the repository root
	args := map[string][]string{
		"lexer":  {"selfhost/lexer/lexer.xo", "selfhost/lexer/token.xo"},
		"parser": {"selfhost/parser/parser.xo"},
		"check":  {"--std=std", "--each", "selfhost/lexer"},
		"ssa":    {"--std=std", "--each", "--stages", "selfhost/lexer"},
		"llvm":   {"--std=std", "--each", "selfhost/lexer"},
	}[comp]
	if args == nil {
		t.Fatalf("XO_SELFHOST_E2E=%s: want lexer, parser, check, ssa, or llvm", comp)
	}
	root := repoRoot(t)
	backend := "go"
	if v := os.Getenv("XO_SELFHOST_BACKEND"); v != "" {
		backend = v
	}
	emitter, err := selfhostBinary("llvm", backend)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := selfhostBinary(comp, "go")
	if err != nil {
		t.Fatal(err)
	}
	dir := "selfhost/" + comp
	t.Chdir(root)
	setModEnv(t, root)
	t0 := time.Now()
	cmd := exec.Command(emitter, "--std=std", "--wd="+workDir(t), dir)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("the Xo emitter: %v", err)
	}
	t.Logf("the self hosted pipeline (%s build) lowered %s in %v", backend, dir, time.Since(t0).Round(time.Millisecond))
	got, ok := strings.CutPrefix(string(out), "== ir\n")
	if !ok {
		t.Fatalf("the Xo emitter prints %s", firstLine(string(out)))
	}
	got, skipped, _ := strings.Cut(got, "== skipped\n")
	if skipped != "" {
		t.Fatalf("the Xo emitter skipped functions:\n%s", skipped)
	}
	want := goLLVM(root, dir, false, string(out))
	if want.missing {
		t.Fatal(want.missingText())
	}
	if n := want.m("none") + want.m("split"); n != "" {
		t.Fatalf("the recording: %s", n)
	}
	if !want.eq("module", got) {
		t.Fatalf("the IR differs from the recording: %s", want.diff("module", got))
	}
	t.Logf("the IR of %s: %d bytes, %d lines, the recorded IR byte for byte", dir, len(got), strings.Count(got, "\n"))
	t0 = time.Now()
	res, err := xotool.Build([]string{dir}, xotool.Options{Backend: xotool.BackendLLVM, Out: filepath.Join(t.TempDir(), comp),
		Env: []string{"XO_BIN_CACHE=off", "XO_BUILD_DIR=" + t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed {
		t.Fatalf("xo build --backend=llvm %s failed:\n%s", dir, res.Output)
	}
	bin := res.Binaries[xotool.Host()]
	t.Logf("xo built %s with the LLVM backend in %v", filepath.Base(bin), time.Since(t0).Round(time.Second))
	run := func(b string) string {
		c := exec.Command(b, args...)
		c.Dir = root
		var o, e bytes.Buffer
		c.Stdout, c.Stderr = &o, &e
		if err := c.Run(); err != nil {
			t.Fatalf("%s: %v\n%s", b, err, e.String())
		}
		return o.String()
	}
	g, x := run(ref), run(bin)
	if g != x {
		t.Fatalf("the binary built from the self hosted IR prints another output than the go build: %s", firstDiff(x, g))
	}
	t.Logf("the binary's output on %v: %d bytes, identical to the go build's", args, len(x))
}
