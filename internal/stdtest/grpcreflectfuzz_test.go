// A fuzz harness for std/grpcreflect's request and response readers against protobuf-go reading grpc-go's
// reflection messages (task #273, decision 0616's fuzz kind). The oracle, internal/stdtest/grpcref's reflectfuzz
// mode, makes random ServerReflectionRequest and ServerReflectionResponse messages and changed copies (cut short, a
// byte changed, two joined, a stray field) and writes what protobuf-go read from each; the Xo program checks
// decode_request and decode_answer against it. TestGrpcReflectFuzz runs XO_GRPCREFLECTFUZZ_SEED (default 2) with
// XO_GRPCREFLECTFUZZ_N cases of each (default 2000) on XO_GRPCREFLECTFUZZ_BACKENDS (default arm64 and llvm); it skips
// when the oracle cannot be built.
package stdtest

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

func TestGrpcReflectFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the grpcreflect fuzz builds the grpc-go oracle")
	}
	seed := envInt(t, "XO_GRPCREFLECTFUZZ_SEED", 2)
	n := envInt(t, "XO_GRPCREFLECTFUZZ_N", 2000)
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := testtmp.Dir("xo-grpcreflectfuzz-")
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
	src, err := exec.Command(ref, "reflectfuzz", strconv.Itoa(seed), strconv.Itoa(n)).Output()
	if err != nil {
		t.Fatalf("reflectfuzz: %v", err)
	}
	runFuzzDir(t, "grpcreflectfuzz_test.xo", string(src), envBackends("XO_GRPCREFLECTFUZZ_BACKENDS", []string{"arm64", "llvm"}), 2*n)
}
