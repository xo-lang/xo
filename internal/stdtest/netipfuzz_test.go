package stdtest

// TestNetipFuzz (task #90): std/netip against Go's net/netip (and net's
// SplitHostPort and JoinHostPort) on seeded texts. IPv4 texts with
// leading zeros, values past 255, too few or many fields, empty fields
// and signs; IPv6 texts with groups of 0 to 5 digits, "::" anywhere and
// twice, upper case, an IPv4 tail (good or not), zones (empty, non-ASCII,
// with a slash), too many groups; prefixes with bits from -2 to 130,
// leading zeros, signs, spaces and nothing; addr:port texts with brackets
// right and wrong and ports past 65535; host:port texts with brackets and
// colons anywhere; 0 to 17 raw bytes; and pairs of addresses to compare.
// A third of the texts then have a byte changed, cut or one inserted.
// Every result must be Go's: the address and each accessor and
// predicate, Prefix(bits), WithZone, Next and Prev; the prefix, Masked,
// Bits, IsSingleIP, Contains and Overlaps; the error texts.
//
//   XO_NETIPFUZZ_SEED (default 2), XO_NETIPFUZZ_N (20000),
//   XO_NETIPFUZZ_BACKENDS (arm64,llvm)

import (
	"bytes"
	"fmt"
	"math/rand"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

func nifPick(r *rand.Rand, xs []string) string { return xs[r.Intn(len(xs))] }

func nifV4(r *rand.Rand) string {
	n := 4
	switch r.Intn(12) {
	case 0:
		n = 3
	case 1:
		n = 5
	}
	f := make([]string, n)
	for i := range f {
		if i < 2 && r.Intn(2) == 0 {
			// the ranges the predicates test, and their edges
			if i == 0 {
				f[i] = nifPick(r, []string{"10", "127", "169", "172", "192", "224", "239", "240", "0", "255", "100", "11", "9"})
			} else {
				f[i] = nifPick(r, []string{"254", "255", "168", "167", "169", "15", "16", "31", "32", "0", "1"})
			}
			continue
		}
		switch r.Intn(20) {
		case 0:
			f[i] = "0" + strconv.Itoa(r.Intn(100))
		case 1:
			f[i] = strconv.Itoa(256 + r.Intn(1000))
		case 2:
			f[i] = ""
		case 3:
			f[i] = "+1"
		case 4:
			f[i] = "00"
		default:
			f[i] = strconv.Itoa(r.Intn(256))
		}
	}
	return strings.Join(f, ".")
}

func nifGroup(r *rand.Rand) string {
	n := 1 + r.Intn(4)
	if r.Intn(25) == 0 {
		n = 5
	}
	if r.Intn(30) == 0 {
		return ""
	}
	h := fmt.Sprintf("%016x", r.Uint64())[:n]
	if r.Intn(5) == 0 {
		h = strings.ToUpper(h)
	}
	if r.Intn(4) == 0 {
		h = "0"
	}
	return h
}

func nifV6(r *rand.Rand) string {
	var s string
	switch r.Intn(6) {
	case 0:
		// with "::" somewhere
		a, b := r.Intn(5), r.Intn(5)
		var g []string
		for i := 0; i < a; i++ {
			g = append(g, nifGroup(r))
		}
		var h []string
		for i := 0; i < b; i++ {
			h = append(h, nifGroup(r))
		}
		s = strings.Join(g, ":") + "::" + strings.Join(h, ":")
	case 1:
		s = nifPick(r, []string{"::", "::1", "::ffff:", "fe80::", "ff02::", "::0.0.0.0", "1::2::3", ":1::", "1:::2"}) + nifPick(r, []string{"", "1", "ffff", "1.2.3.4"})
	case 2:
		var g []string
		for i := 0; i < 6; i++ {
			g = append(g, nifGroup(r))
		}
		s = strings.Join(g, ":") + ":" + nifV4(r)
	default:
		n := 8
		switch r.Intn(10) {
		case 0:
			n = 7
		case 1:
			n = 9
		}
		var g []string
		for i := 0; i < n; i++ {
			g = append(g, nifGroup(r))
		}
		if r.Intn(2) == 0 && len(g) > 0 {
			// the ranges the predicates test, and their edges
			g[0] = nifPick(r, []string{"fe80", "febf", "fec0", "fc00", "fdff", "fe00", "ff02", "ff00", "feff", "0", "2001"})
		}
		s = strings.Join(g, ":")
	}
	if r.Intn(6) == 0 {
		s += "%" + nifPick(r, []string{"eth0", "", "1", "über", "a/b", "%", "en0 x"})
	}
	return s
}

func nifAddr(r *rand.Rand) string {
	switch r.Intn(10) {
	case 0:
		return nifPick(r, []string{"", "localhost", "1.2.3", "::ffff:127.0.0.1", "0.0.0.0", "255.255.255.255", "::", "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff", "::ffff:255.255.255.255", "127.0.0.1", "10.0.0.1", "fc00::1", "169.254.1.1", "fe80::1%eth0", "224.0.0.1", "ff02::1"})
	case 1, 2, 3:
		return nifV4(r)
	}
	return nifV6(r)
}

// nifChange: a third of the texts with a byte changed, cut or inserted.
func nifChange(r *rand.Rand, s string) string {
	if r.Intn(3) != 0 || s == "" {
		return s
	}
	alpha := []string{"0", "9", "a", "f", "g", "A", ":", ".", "%", "/", "[", "]", " ", "-", "+", "ü", "x"}
	i := r.Intn(len(s))
	switch r.Intn(3) {
	case 0:
		s = s[:i] + nifPick(r, alpha) + s[i+1:]
	case 1:
		s = s[:i]
	default:
		s = s[:i] + nifPick(r, alpha) + s[i:]
	}
	return strings.ToValidUTF8(s, "�")
}

func nifBits(r *rand.Rand) string {
	switch r.Intn(12) {
	case 0:
		return "0" + strconv.Itoa(r.Intn(10))
	case 1:
		return ""
	case 2:
		return "+8"
	case 3:
		return " 8"
	case 4:
		return "-1"
	}
	return strconv.Itoa(r.Intn(131))
}

func nifHost(r *rand.Rand) string {
	switch r.Intn(5) {
	case 0:
		return nifPick(r, []string{"example.com", "localhost", "", "a:b", "[::1", "::1]", "[]"})
	case 1:
		return "[" + nifV6(r) + "]"
	}
	return nifAddr(r)
}

func nifPort(r *rand.Rand) string {
	return nifPick(r, []string{"80", "0", "65535", "65536", "", "08", "-1", "http", "70000", "1" + strings.Repeat("0", 6), "443", "0x10", " 22"})
}

func nifHex(s string) string { return fmt.Sprintf("x%x", s) }

// nifGen: one case as a line for the Xo client.
func nifGen(r *rand.Rand) string {
	switch k := r.Intn(12); {
	case k < 4:
		return fmt.Sprintf("addr %s %s %s", nifHex(nifChange(r, nifAddr(r))), nifHex(strconv.Itoa(r.Intn(133)-2)), nifHex(nifPick(r, []string{"", "eth0", "zü"})))
	case k < 7:
		p := nifChange(r, nifAddr(r)+"/"+nifBits(r))
		q := nifAddr(r) + "/" + strconv.Itoa(r.Intn(129))
		return fmt.Sprintf("prefix %s %s %s", nifHex(p), nifHex(nifAddr(r)), nifHex(q))
	case k < 8:
		h := nifAddr(r)
		if strings.Contains(h, ":") && r.Intn(5) != 0 || r.Intn(10) == 0 {
			h = "[" + h + "]"
		}
		return fmt.Sprintf("addrport %s", nifHex(nifChange(r, h+":"+nifPort(r))))
	case k < 10:
		return fmt.Sprintf("split %s", nifHex(nifChange(r, nifHost(r)+nifPick(r, []string{":", ":", "", "::", "]:"})+nifPort(r))))
	case k < 11:
		n := r.Intn(18)
		if r.Intn(2) == 0 {
			n = []int{4, 16}[r.Intn(2)]
		}
		b := make([]byte, n)
		r.Read(b)
		if n == 16 && r.Intn(3) == 0 {
			copy(b, make([]byte, 10))
			b[10], b[11] = 0xff, 0xff
		}
		return fmt.Sprintf("from x%x", b)
	default:
		return fmt.Sprintf("cmp %s %s", nifHex(nifAddr(r)), nifHex(nifAddr(r)))
	}
}

func nifUnhex(h string) string {
	var b []byte
	fmt.Sscanf(strings.TrimPrefix(h, "x"), "%x", &b)
	return string(b)
}

func nifB01(x bool) string {
	if x {
		return "1"
	}
	return "0"
}

func nifDescribe(a netip.Addr) string {
	flags := nifB01(a.Is4()) + nifB01(a.Is6()) + nifB01(a.Is4In6()) + nifB01(a.IsLoopback()) + nifB01(a.IsPrivate()) + nifB01(a.IsUnspecified()) + nifB01(a.IsLinkLocalUnicast()) + nifB01(a.IsMulticast())
	return fmt.Sprintf("%s bits %d zone %s bytes %x flags %s unmap %s", a, a.BitLen(), strconv.Quote(a.Zone()), a.AsSlice(), flags, a.Unmap())
}

func nifOpt(a netip.Addr) string {
	if !a.IsValid() {
		return "none"
	}
	return a.String()
}

// nifGo: Go's results of a case line, as the Xo client prints them.
func nifGo(line string) string {
	w := strings.Split(line, " ")
	var b strings.Builder
	switch w[0] {
	case "addr":
		a, err := netip.ParseAddr(nifUnhex(w[1]))
		if err != nil {
			return "error " + err.Error() + "\n"
		}
		b.WriteString(nifDescribe(a) + "\n")
		bits, _ := strconv.Atoi(nifUnhex(w[2]))
		if p, err := a.Prefix(bits); err != nil {
			fmt.Fprintf(&b, "prefix %d error %s\n", bits, err)
		} else {
			fmt.Fprintf(&b, "prefix %d %s\n", bits, p)
		}
		fmt.Fprintf(&b, "zone %s\n", nifDescribe(a.WithZone(nifUnhex(w[3]))))
		fmt.Fprintf(&b, "next %s prev %s\n", nifOpt(a.Next()), nifOpt(a.Prev()))
	case "prefix":
		p, err := netip.ParsePrefix(nifUnhex(w[1]))
		if err != nil {
			return "error " + err.Error() + "\n"
		}
		fmt.Fprintf(&b, "%s masked %s bits %d single %s\n", p, p.Masked(), p.Bits(), nifB01(p.IsSingleIP()))
		if a, err := netip.ParseAddr(nifUnhex(w[2])); err == nil {
			fmt.Fprintf(&b, "contains %s %s\n", a, nifB01(p.Contains(a)))
		}
		if q, err := netip.ParsePrefix(nifUnhex(w[3])); err == nil {
			fmt.Fprintf(&b, "overlaps %s %s\n", q, nifB01(p.Overlaps(q)))
		}
	case "addrport":
		ap, err := netip.ParseAddrPort(nifUnhex(w[1]))
		if err != nil {
			return "error " + err.Error() + "\n"
		}
		fmt.Fprintf(&b, "%s addr %s port %d\n", ap, ap.Addr(), ap.Port())
	case "split":
		h, p, err := net.SplitHostPort(nifUnhex(w[1]))
		if err != nil {
			return "error " + err.Error() + "\n"
		}
		fmt.Fprintf(&b, "host %s port %s join %s\n", strconv.Quote(h), strconv.Quote(p), strconv.Quote(net.JoinHostPort(h, p)))
	case "from":
		a, ok := netip.AddrFromSlice([]byte(nifUnhex(w[1])))
		if !ok {
			return "none\n"
		}
		b.WriteString(nifDescribe(a) + "\n")
	default:
		x, e1 := netip.ParseAddr(nifUnhex(w[1]))
		y, e2 := netip.ParseAddr(nifUnhex(w[2]))
		if e1 != nil || e2 != nil {
			return "cmp skip\n"
		}
		fmt.Fprintf(&b, "cmp %d\n", x.Compare(y))
	}
	return b.String()
}

// nifBlocks: the output of each case in order (each starts with "== ").
func nifBlocks(out string) []string {
	var bs []string
	for _, l := range strings.Split(out, "\n") {
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

func nifBuild(t *testing.T, work, backend string) string {
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(work, "xo-netip-"+backend)
	args := []string{"build"}
	if backend != "" {
		args = append(args, "--backend="+backend)
	}
	args = append(args, "-o", bin, filepath.Join(root, "scripts", "netipdiff", "xo"))
	xb := testtmp.Guard(exec.Command(xo, args...))
	xb.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
	if out, err := xb.CombinedOutput(); err != nil {
		t.Fatalf("xo build --backend=%s: %v\n%s", backend, err, out)
	}
	return bin
}

func nifRunXo(t *testing.T, bin string, lines []string) []string {
	cmd := testtmp.Guard(exec.Command(bin))
	cmd.Stdin = strings.NewReader(strings.Join(lines, "\n") + "\n")
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s: %v\n%s\nstderr:\n%s", bin, err, myfTail(out.String(), 20), myfTail(errb.String(), 30))
	}
	return nifBlocks(out.String())
}

// nifShow: a case line with its texts decoded, for a failure's report.
func nifShow(line string) string {
	w := strings.Split(line, " ")
	for i := 1; i < len(w); i++ {
		s := nifUnhex(w[i])
		if utf8.ValidString(s) && w[0] != "from" {
			w[i] = strconv.Quote(s)
		}
	}
	return strings.Join(w, " ")
}

func TestNetipFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the netip fuzz builds the Xo client")
	}
	seed := int64(envInt(t, "XO_NETIPFUZZ_SEED", 2))
	n := envInt(t, "XO_NETIPFUZZ_N", 20000)
	backends := envBackends("XO_NETIPFUZZ_BACKENDS", []string{"arm64", "llvm"})
	r := rand.New(rand.NewSource(seed))
	lines := make([]string, n)
	want := make([]string, n)
	errs := 0
	for i := range lines {
		lines[i] = nifGen(r)
		want[i] = nifGo(lines[i])
		if strings.HasPrefix(want[i], "error ") {
			errs++
		}
	}
	t.Logf("seed %d: %d cases, %d of them errors in Go", seed, n, errs)
	work, err := testtmp.Dir("xo-netipfuzz-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	for _, b := range backends {
		got := nifRunXo(t, nifBuild(t, work, b), lines)
		if len(got) != n {
			t.Fatalf("%s: %d results for %d cases", b, len(got), n)
		}
		bad := 0
		for i := 0; i < n; i++ {
			if got[i] == want[i] {
				continue
			}
			if bad < 10 {
				t.Errorf("%s case %d (seed %d): %s\ngot\n%swant\n%s", b, i, seed, nifShow(lines[i]), got[i], want[i])
			}
			bad++
		}
		if bad > 0 {
			t.Fatalf("%s: %d of %d cases differ (seed %d)", b, bad, n, seed)
		}
		t.Logf("%s: %d cases equal Go's net/netip (seed %d)", b, n, seed)
	}
}
