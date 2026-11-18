// A fuzz harness for std/grpchealth's readers against protobuf-go reading grpc-go's health messages (#152, decision
// 0616's fuzz kind). The oracle, internal/stdtest/grpcref's healthfuzz mode, makes random HealthCheckRequest,
// HealthCheckResponse and HealthListResponse messages and changed copies (cut short, a byte changed, two joined, a
// stray field) and writes what protobuf-go read from each; the Xo program checks check, list and the health server
// against it. TestGrpcHealthFuzz runs XO_GRPCHEALTHFUZZ_SEED (default 2) with XO_GRPCHEALTHFUZZ_N cases of each
// (default 2000) on XO_GRPCHEALTHFUZZ_BACKENDS (default arm64 and llvm); it skips when the oracle cannot be built.
package stdtest

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

func TestGrpcHealthFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the grpchealth fuzz builds the grpc-go oracle")
	}
	seed := envInt(t, "XO_GRPCHEALTHFUZZ_SEED", 2)
	n := envInt(t, "XO_GRPCHEALTHFUZZ_N", 2000)
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := testtmp.Dir("xo-grpchealthfuzz-")
	if err != nil {
		t.Fatal(err)
	}
	ref := filepath.Join(dir, "grpcref")
	b := exec.Command("go", "build", "-o", ref, ".")
	b.Dir = filepath.Join(root, "internal", "stdtest", "grpcref")
	b.Env = append(b.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off")
	if out, err := b.CombinedOutput(); err != nil {
		t.Skipf("the grpc-go oracle cannot be built here: %v\n%s", err, out)
	}
	src, err := exec.Command(ref, "healthfuzz", strconv.Itoa(seed), strconv.Itoa(n)).Output()
	if err != nil {
		t.Fatalf("healthfuzz: %v", err)
	}
	runFuzzDir(t, "grpchealthfuzz_test.xo", string(src), envBackends("XO_GRPCHEALTHFUZZ_BACKENDS", []string{"arm64", "llvm"}), 3*n, "--real")
}
