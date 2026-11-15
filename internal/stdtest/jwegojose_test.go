// The reverse direction of std/jwt's JWE corpus (decision 0660, accepted on
// the condition that go-jose reads what std/jwt writes; task #39): an Xo
// program printed by scripts/stdcorpus/jweoracle encrypts a payload under
// every key management and content encryption combination with jwt.encrypt,
// and jweoracle decrypts each token with github.com/go-jose/go-jose/v4,
// checking the payload and the kid. It runs on XO_JWEGOJOSE_BACKENDS
// (default arm64, llvm and go) outside -short, and is skipped when the Go
// module cache has no go-jose (the oracle is its own module, so go.mod stays
// without dependencies).
package stdtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

func TestJweGoJoseDecrypts(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a program on each backend; the JWE corpus runs in TestStdCorpora")
	}
	oracle := filepath.Join("..", "..", "scripts", "stdcorpus", "jweoracle")
	gen := exec.Command("go", "run", ".", "reverse")
	gen.Dir = oracle
	prog, err := gen.Output()
	if err != nil {
		t.Skipf("jweoracle cannot run (go-jose not in the module cache?): %v", err)
	}
	combos := strings.Count(string(prog), "(jwt.KeyAlg.")
	if combos < 90 {
		t.Fatalf("the program holds %d combinations", combos)
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-jwegojose-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	if err := os.WriteFile(filepath.Join(work, "main.xo"), prog, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, b := range envBackends("XO_JWEGOJOSE_BACKENDS", []string{"arm64", "llvm", "go"}) {
		t.Run(b, func(t *testing.T) {
			run := testtmp.Guard(exec.Command(xo, "run", "--backend="+b, work))
			run.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
			var stderr strings.Builder
			run.Stderr = &stderr
			toks, err := run.Output()
			if err != nil {
				t.Fatalf("xo run --backend=%s: %v\n%s", b, err, stderr.String())
			}
			if n := strings.Count(string(toks), "\n"); n != combos {
				t.Fatalf("%d tokens for %d combinations", n, combos)
			}
			dec := exec.Command("go", "run", ".", "decrypt")
			dec.Dir = oracle
			dec.Stdin = strings.NewReader(string(toks))
			out, err := dec.CombinedOutput()
			if err != nil {
				t.Fatalf("go-jose does not decrypt std/jwt's tokens (%s): %v\n%s", b, err, out)
			}
			t.Logf("%s: %s", b, strings.TrimSpace(string(out)))
		})
	}
}
