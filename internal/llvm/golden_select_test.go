package llvm

// A gate runs the LLVM goldens its change touches (owner, 2026-10-22: the
// whole package cost 46 to 56 minutes a gate): scripts/gate.sh's llvm
// step compares TestGoldenIRManifest's hashes of every golden's IR, made
// by the branch's xo and by its merge base's, and names the goldens whose
// IR or directory changed in XO_LLVM_GOLDENS.

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// goldenSelected reports whether golden program name runs in TestGolden,
// TestGoldenSplit, TestGoldenGoParity and TestDebugInfoSameOutputLLVM: every one when XO_LLVM_GOLDENS
// is unset; none when it is "none"; else the comma separated names, and
// o2Goldens with them (the -O2 sample runs when any golden changed).
func goldenSelected(name string) bool {
	v, ok := os.LookupEnv("XO_LLVM_GOLDENS")
	if !ok {
		return true
	}
	if v == "none" {
		return false
	}
	for _, n := range strings.Split(v, ",") {
		if strings.TrimSpace(n) == name {
			return true
		}
	}
	return o2Goldens[name]
}

// skipUnselected skips a golden's subtest that XO_LLVM_GOLDENS leaves out.
func skipUnselected(t *testing.T, dir string) {
	t.Helper()
	if !goldenSelected(filepath.Base(dir)) {
		t.Skip("not selected (XO_LLVM_GOLDENS: its IR and directory are unchanged)")
	}
}

// stdPos matches a source position in a std file in a fault location
// string (std/native/prelude.xo:672:5); debugPos a line or column of the
// debug metadata.
var (
	stdPos   = regexp.MustCompile(`(std/[A-Za-z0-9_./-]+\.xo):[0-9]+(:[0-9]+)?`)
	debugPos = regexp.MustCompile(`\b(line|column|scopeLine): [0-9]+`)
)

// normalizeIR is the IR a golden's manifest hashes: the repository's
// path root replaced, and the lines and columns of std files and of the
// debug metadata dropped. A golden's output names only its own main.xo's
// positions (the fault location strings, kept), so a std change that
// moves lines (a function added to the prelude) leaves the other goldens
// unselected; on 2026-10-22 Str.clone moved the prelude's lines and
// changed every golden's IR in nothing else.
func normalizeIR(ir, root string) string {
	ir = strings.ReplaceAll(ir, root, "$ROOT")
	ir = stdPos.ReplaceAllString(ir, "$1:L")
	return debugPos.ReplaceAllString(ir, "$1: L")
}

// TestGoldenIRManifest writes, to $XO_LLVM_IR_MANIFEST, one line per
// golden program: its name and the SHA-256 of the LLVM IR xo emits for it
// with TestGolden's options (goldenEnv), clang never run (a stub that
// fails after the IR is written), normalized (normalizeIR) so two
// worktrees agree. A program whose build fails before the IR is
// hashed by its output instead. scripts/gate.sh runs it with the branch's
// xo and with the merge base's.
func TestGoldenIRManifest(t *testing.T) {
	out := os.Getenv("XO_LLVM_IR_MANIFEST")
	if out == "" {
		t.Skip("XO_LLVM_IR_MANIFEST=FILE writes the manifest (scripts/gate.sh's llvm step)")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	dirs, err := filepath.Glob(filepath.Join("testdata", "golden", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no golden programs: %v", err)
	}
	tmp := t.TempDir()
	stub := filepath.Join(tmp, "noclang")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	hashes := map[string]string{}
	t.Run("ir", func(t *testing.T) {
		for _, dir := range dirs {
			dir := dir
			name := filepath.Base(dir)
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				ll := filepath.Join(tmp, name+".ll")
				env := append(goldenEnv(dir), "XO_CLANG="+stub, "XO_BIN_CACHE=off")
				res, err := xotool.Build([]string{dir}, xotool.Options{Backend: xotool.BackendLLVM, Flags: []string{"--asm=" + ll}, Env: env, Out: filepath.Join(tmp, name)})
				if err != nil {
					t.Fatal(err)
				}
				var h string
				if b, err := os.ReadFile(ll); err == nil {
					h = fmt.Sprintf("%x", sha256.Sum256([]byte(normalizeIR(string(b), root))))
					os.Remove(ll)
				} else {
					h = fmt.Sprintf("noir-%x", sha256.Sum256([]byte(strings.ReplaceAll(res.Output, root, "$ROOT"))))
				}
				mu.Lock()
				hashes[name] = h
				mu.Unlock()
			})
		}
	})
	names := make([]string, 0, len(hashes))
	for n := range hashes {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		fmt.Fprintf(&b, "%s %s\n", n, hashes[n])
	}
	if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}
