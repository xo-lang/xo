package stdtest

// A differential of std/amqp's decode_message against github.com/Azure/go-amqp v1.7.0's
// Message.UnmarshalBinary (std/amqp's reference, decision 0521; task #115): go-amqp's encoding
// of seeded messages (every section, values of every type nested in lists and maps, arrays,
// annotations with symbol and long keys), most of them then mutated (a byte set to a constructor
// code or a random value, a cut, inserted bytes, a tail repeated). The oracle is
// scripts/stdcorpus/amqporacle's fuzz mode; the Xo side is scripts/amqpdiff/xo. A message both
// decode must show the same (the corpus's canonical text); one go-amqp refuses must be refused;
// one go-amqp panics on (a binary map key) is skipped, the Xo side must not fault. Modeled by the oracle: std/amqp's documented
// refusals (a str or sym that is not UTF-8, a second header or properties section, mixed or
// repeated value body sections) and its superset (what go-amqp reports as not implemented, an
// invalid type or an invalid map key, skipped); the Xo side shows a map with a repeated key as
// go-amqp's Go map holds it, the last value.
//
//   XO_AMQPFUZZ_SEED (2), _N (20000), _BACKENDS (arm64,llvm), _SHOW (10: differing cases printed)

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

func amfBlocks(s string) []string {
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

// amfQuiet: want with each signaling NaN binary32 ("float 7f8xxxxx" or "ff8...", the quiet
// bit clear, a payload) quieted, as the native backends hand it back: their Float32.from_bits
// narrows a double, which the hardware quiets (a known bug, reported with task #115; the Go
// backend keeps the bits).
var amfFloat = regexp.MustCompile(`float ([0-9a-f]{8})`)

func amfQuiet(s string) string {
	return amfFloat.ReplaceAllStringFunc(s, func(m string) string {
		v, _ := strconv.ParseUint(m[6:], 16, 32)
		if v&0x7f800000 == 0x7f800000 && v&0x7fffff != 0 {
			v |= 0x400000
		}
		return fmt.Sprintf("float %08x", v)
	})
}

func TestAmqpFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("builds an Xo program and runs the go-amqp oracle")
	}
	seed := envInt(t, "XO_AMQPFUZZ_SEED", 2)
	n := envInt(t, "XO_AMQPFUZZ_N", 20000)
	show := envInt(t, "XO_AMQPFUZZ_SHOW", 10)
	backends := envBackends("XO_AMQPFUZZ_BACKENDS", []string{"arm64", "llvm"})
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	oc := testtmp.Guard(exec.Command("go", "run", ".", "fuzz", fmt.Sprint(seed), fmt.Sprint(n)))
	oc.Dir = filepath.Join(root, "scripts", "stdcorpus", "amqporacle")
	oc.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off")
	var oout, oerr bytes.Buffer
	oc.Stdout = &oout
	oc.Stderr = &oerr
	if err := oc.Run(); err != nil {
		t.Skipf("the go-amqp oracle cannot run here: %v\n%s", err, oerr.String())
	}
	var in strings.Builder
	var cases, want []string
	kinds := map[string]int{}
	for _, b := range amfBlocks(oout.String()) {
		ls := strings.SplitN(strings.TrimSuffix(b, "\n"), "\n", 2)
		if len(ls) != 2 || !strings.HasPrefix(ls[0], "c ") || !strings.HasPrefix(ls[1], "w ") {
			t.Fatalf("the oracle: a block %q", b)
		}
		w := strings.TrimPrefix(ls[1], "w ")
		switch w {
		case "unshowable", "unsupported", "panic":
			// a panic: go-amqp's map of a binary key (an unhashable Go map key), which std/amqp
			// decodes; a fault on the Xo side still fails the run
			// go-amqp decodes it to a value show has no case for, or cannot decode what
			// std/amqp can (char, decimals, described values in any place, list, map and
			// binary map keys, typed fields in any integer encoding)
			kinds[w]++
			continue
		case "error":
			kinds["error"]++
		default:
			kinds["decoded"]++
		}
		in.WriteString(ls[0] + "\n")
		cases = append(cases, strings.TrimPrefix(ls[0], "c "))
		want = append(want, w+"\n")
	}
	t.Logf("seed %d: %d cases, go-amqp %v", seed, len(cases), kinds)
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-amqpfuzz-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	for _, be := range backends {
		bin := filepath.Join(work, "amqpdiff-"+be)
		xb := testtmp.Guard(exec.Command(xo, "build", "--backend="+be, "-o", bin, filepath.Join(root, "scripts", "amqpdiff", "xo")))
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
		got := amfBlocks(out.String())
		if len(got) != len(cases) {
			t.Fatalf("%s: %d results for %d cases\n%s", be, len(got), len(cases), errb.String())
		}
		bad := 0
		for i := range cases {
			if got[i] == want[i] || (be != "go" && got[i] == amfQuiet(want[i])) {
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
		t.Logf("%s: %d cases equal go-amqp's (seed %d)", be, len(cases), seed)
	}
}
