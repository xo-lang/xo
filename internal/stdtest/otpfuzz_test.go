package stdtest

// A differential of std/otp against github.com/pquerna/otp v1.5.0 (std/otp's reference, decision
// 0473; task #117) on seeded cases (scripts/stdcorpus/otporacle): HOTP and TOTP codes (secrets of
// 0 to 200 bytes, counters and times to Int's edge, decimal and Steam), verify against
// totp.ValidateCustom on near and changed codes, parse_uri and secret_bytes against NewKeyFromURL
// and the Key's methods on otpauth URIs most of them mutated, uri and hotp_uri against
// totp.Generate and hotp.Generate, and Key.image against Key.Image (its size and the sha256 of its
// pixels). The Xo side is scripts/otpdiff/xo. The oracle skips (and std/otp's doc lists) the
// departures: a step past 2^53 (pquerna floors a float64 quotient), a code with white space
// around it (pquerna trims it), a step under 0 (pquerna wraps it), a period or digits past Int.
//
//   XO_OTPFUZZ_SEED (2), _N (20000), _BACKENDS (arm64,llvm), _SHOW (10: differing cases printed)

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

func otfBlocks(s string) []string {
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

func TestOtpFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("builds an Xo program and runs the pquerna/otp oracle")
	}
	seed := envInt(t, "XO_OTPFUZZ_SEED", 2)
	n := envInt(t, "XO_OTPFUZZ_N", 20000)
	show := envInt(t, "XO_OTPFUZZ_SHOW", 10)
	backends := envBackends("XO_OTPFUZZ_BACKENDS", []string{"arm64", "llvm"})
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	oc := testtmp.Guard(exec.Command("go", "run", ".", fmt.Sprint(seed), fmt.Sprint(n)))
	oc.Dir = filepath.Join(root, "scripts", "stdcorpus", "otporacle")
	oc.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off")
	var oout, oerr bytes.Buffer
	oc.Stdout = &oout
	oc.Stderr = &oerr
	if err := oc.Run(); err != nil {
		t.Skipf("the pquerna/otp oracle cannot run here: %v\n%s", err, oerr.String())
	}
	var in strings.Builder
	var cases, want []string
	kinds := map[string]int{}
	for _, b := range otfBlocks(oout.String()) {
		ls := strings.SplitN(strings.TrimSuffix(b, "\n"), "\n", 2)
		if len(ls) != 2 || !strings.HasPrefix(ls[0], "c ") || !strings.HasPrefix(ls[1], "w ") {
			t.Fatalf("the oracle: a block %q", b)
		}
		w := strings.TrimPrefix(ls[1], "w ")
		kind := strings.TrimPrefix(ls[0], "c ")[:1]
		if w == "skip" {
			kinds[kind+" skip"]++
			continue
		}
		kinds[kind]++
		in.WriteString(ls[0] + "\n")
		cases = append(cases, strings.TrimPrefix(ls[0], "c "))
		want = append(want, w+"\n")
	}
	t.Logf("seed %d: %d cases, pquerna/otp %v", seed, len(cases), kinds)
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-otpfuzz-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	for _, be := range backends {
		bin := filepath.Join(work, "otpdiff-"+be)
		xb := testtmp.Guard(exec.Command(xo, "build", "--backend="+be, "-o", bin, filepath.Join(root, "scripts", "otpdiff", "xo")))
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
		got := otfBlocks(out.String())
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
		t.Logf("%s: %d cases equal pquerna/otp's (seed %d)", be, len(cases), seed)
	}
}
