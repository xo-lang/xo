package stdtest

// A differential of std/httpproxy against golang.org/x/net/http/httpproxy v0.62.0 (task #115) on
// seeded cases (scripts/stdcorpus/httpproxyoracle): HTTP_PROXY and HTTPS_PROXY settings (bare
// hosts, schemes, userinfo, IPv6, ports, bogus ones), NO_PROXY lists of names, wildcards, IPs,
// CIDR blocks and ports, many mutated, CGI, and http and https request URLs, Config.proxy_func
// against Config.ProxyFunc. The Xo side is scripts/httpproxydiff/xo. The oracle skips std/httpproxy's
// documented departures: a proxy setting Go refuses with an error or one of another scheme than
// http and https (Xo ignores it), and a request URL net/url cannot parse.
//
//   XO_HTTPPROXYFUZZ_SEED (2), _N (20000), _BACKENDS (arm64,llvm), _SHOW (10: differing cases printed)

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

func hpfBlocks(s string) []string {
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

func TestHttpproxyFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("builds an Xo program and runs the x/net httpproxy oracle")
	}
	seed := envInt(t, "XO_HTTPPROXYFUZZ_SEED", 2)
	n := envInt(t, "XO_HTTPPROXYFUZZ_N", 20000)
	show := envInt(t, "XO_HTTPPROXYFUZZ_SHOW", 10)
	backends := envBackends("XO_HTTPPROXYFUZZ_BACKENDS", []string{"arm64", "llvm"})
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	oc := testtmp.Guard(exec.Command("go", "run", ".", fmt.Sprint(seed), fmt.Sprint(n)))
	oc.Dir = filepath.Join(root, "scripts", "stdcorpus", "httpproxyoracle")
	oc.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off")
	var oout, oerr bytes.Buffer
	oc.Stdout = &oout
	oc.Stderr = &oerr
	if err := oc.Run(); err != nil {
		t.Skipf("the x/net httpproxy oracle cannot run here: %v\n%s", err, oerr.String())
	}
	var in strings.Builder
	var cases, want []string
	kinds := map[string]int{}
	for _, b := range hpfBlocks(oout.String()) {
		ls := strings.SplitN(strings.TrimSuffix(b, "\n"), "\n", 2)
		if len(ls) != 2 || !strings.HasPrefix(ls[0], "c ") || !strings.HasPrefix(ls[1], "w ") {
			t.Fatalf("the oracle: a block %q", b)
		}
		w := strings.TrimPrefix(ls[1], "w ")
		kind := strings.SplitN(w, " ", 2)[0]
		if w == "skip" {
			kinds["skip"]++
			continue
		}
		kinds[kind]++
		in.WriteString(ls[0] + "\n")
		cases = append(cases, strings.TrimPrefix(ls[0], "c "))
		want = append(want, w+"\n")
	}
	t.Logf("seed %d: %d cases, x/net httpproxy %v", seed, len(cases), kinds)
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-httpproxyfuzz-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	for _, be := range backends {
		bin := filepath.Join(work, "httpproxydiff-"+be)
		xb := testtmp.Guard(exec.Command(xo, "build", "--backend="+be, "-o", bin, filepath.Join(root, "scripts", "httpproxydiff", "xo")))
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
		got := hpfBlocks(out.String())
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
		t.Logf("%s: %d cases equal x/net httpproxy's (seed %d)", be, len(cases), seed)
	}
}
