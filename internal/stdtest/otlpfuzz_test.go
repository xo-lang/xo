package stdtest

// A differential of std/otlp against go.opentelemetry.io/proto/otlp v1.11.1 (protobuf-go, deterministic
// Marshal) and otlptracehttp v1.47.0's JSON encoder (task #115) on seeded cases
// (scripts/stdcorpus/otlporacle): a Resource (service, attributes, scope) and 0 to 4 std/trace spans
// with given, derived and malformed trace, span and parent ids, every kind and status rule, HTTP
// statuses, attributes of every value kind (NaN and the infinities among the floats), events, scopes
// and schema URLs. The oracle builds the ExportTraceServiceRequest from std/otlp's documented rules;
// encode_traces and encode_traces_json must match its bytes. The Xo side is scripts/otlpdiff/xo.
//
//   XO_OTLPFUZZ_SEED (2), _N (5000), _BACKENDS (arm64,llvm), _SHOW (10: differing cases printed)

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

func otlpBlocks(s string) []string {
	var bs []string
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(l, "== ") {
			bs = append(bs, "")
			continue
		}
		if len(bs) > 0 && l != "" {
			bs[len(bs)-1] += l + "\n"
		}
	}
	return bs
}

func TestOtlpFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("builds an Xo program and runs the otlp oracle")
	}
	seed := envInt(t, "XO_OTLPFUZZ_SEED", 2)
	n := envInt(t, "XO_OTLPFUZZ_N", 5000)
	show := envInt(t, "XO_OTLPFUZZ_SHOW", 10)
	backends := envBackends("XO_OTLPFUZZ_BACKENDS", []string{"arm64", "llvm"})
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	oc := testtmp.Guard(exec.Command("go", "run", ".", fmt.Sprint(seed), fmt.Sprint(n)))
	oc.Dir = filepath.Join(root, "scripts", "stdcorpus", "otlporacle")
	oc.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off")
	var oout, oerr bytes.Buffer
	oc.Stdout = &oout
	oc.Stderr = &oerr
	if err := oc.Run(); err != nil {
		t.Skipf("the otlp oracle cannot run here: %v\n%s", err, oerr.String())
	}
	var in strings.Builder
	var cases, want []string
	kinds := map[string]int{}
	for _, b := range otlpBlocks(oout.String()) {
		ls := strings.SplitN(strings.TrimSuffix(b, "\n"), "\n", 2)
		if len(ls) != 2 || !strings.HasPrefix(ls[0], "c ") || !strings.HasPrefix(ls[1], "w ") {
			t.Fatalf("the oracle: a block %q", b)
		}
		w := ls[1]
		kind := "cases"
		kinds[kind]++
		in.WriteString(ls[0] + "\n")
		cases = append(cases, strings.TrimPrefix(ls[0], "c "))
		want = append(want, w+"\n")
	}
	t.Logf("seed %d: %d cases, otlp %v", seed, len(cases), kinds)
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-otlpfuzz-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	for _, be := range backends {
		bin := filepath.Join(work, "otlpdiff-"+be)
		xb := testtmp.Guard(exec.Command(xo, "build", "--backend="+be, "-o", bin, filepath.Join(root, "scripts", "otlpdiff", "xo")))
		xb.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
		if out, err := xb.CombinedOutput(); err != nil {
			t.Fatalf("xo build --backend=%s: %v\n%s", be, err, out)
		}
		cmd := testtmp.Guard(exec.Command(bin))
		cmd.Stdin = strings.NewReader(in.String())
		var out, errb bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &errb
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s: %v\n%s", be, err, errb.String())
		}
		got := otlpBlocks(out.String())
		if len(got) != len(cases) {
			t.Fatalf("%s: %d results for %d cases\n%s", be, len(got), len(cases), errb.String())
		}
		bad := 0
		for i := range cases {
			if got[i] == want[i] {
				continue
			}
			if bad < show {
				t.Errorf("%s case %d (seed %d): %s\ngot  %swant %s", be, i, seed, cases[i], got[i], want[i])
			}
			bad++
		}
		if bad > 0 {
			t.Fatalf("%s: %d of %d cases differ (seed %d)", be, bad, len(cases), seed)
		}
		t.Logf("%s: %d cases equal the oracle's (seed %d)", be, len(cases), seed)
	}
}
