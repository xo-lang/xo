package repotest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGoldenAsmOwnsInputs: internal/arm64's TestGoldenAsm compares the
// assembly of two programs with golden files, so the programs are the
// test's own (internal/arm64/testdata/asm), not the benchmarks'. It read
// bench/perf/llvm/xo/fib and sieve, and 1ed49a3e3f (task #686), which
// gave every benchmark a repeat count, left the four goldens stale with
// no change to the compiler: the 0.55.1 candidate's tier failed on them
// (task #698, gotchas/golden-test-on-bench-sources.md).
func TestGoldenAsmOwnsInputs(t *testing.T) {
	rel := filepath.Join("internal", "arm64", "arm64_test.go")
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	at := strings.Index(src, "\nfunc TestGoldenAsm(")
	if at < 0 {
		t.Fatalf("%s: no func TestGoldenAsm", rel)
	}
	body := src[at+1:]
	if end := strings.Index(body, "\n}\n"); end >= 0 {
		body = body[:end]
	}
	if strings.Contains(body, "benchDir") || strings.Contains(body, `"bench"`) {
		t.Errorf("%s: TestGoldenAsm reads a program under bench/: a benchmark edit then rewrites its goldens; compile the copies under internal/arm64/testdata/asm (asmDir)", rel)
	}
	if !strings.Contains(body, "asmDir") {
		t.Errorf("%s: TestGoldenAsm does not compile from asmDir", rel)
	}
	for _, name := range []string{"fib", "sieve"} {
		for _, f := range []string{"main.xo", "xo.mod"} {
			p := filepath.Join("internal", "arm64", "testdata", "asm", name, f)
			if _, err := os.Stat(filepath.Join(root, p)); err != nil {
				t.Errorf("%s: the test's own copy is missing: %v", p, err)
			}
		}
	}
}
