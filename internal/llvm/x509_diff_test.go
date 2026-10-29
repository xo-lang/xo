package llvm

import (
	"bytes"
	"encoding/hex"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// x509DiffProgram parses each line of its input (a certificate's DER in
// hex) with internal.crypto_op's x509.check and prints the result in hex:
// Go's x509.ParseCertificate on the Go backend, std/native/llrt/x509cert.xo
// on the native ones.
const x509DiffProgram = `use std/internal

fn main(os: Os) {
  let all = os.stdio.read_all().or_fault("stdin")
  for line in all.lines() {
    if line != "" {
      let r = internal.crypto_op("x509.check", Bytes.from_hex(line) ?? b"", b"", b"", b"")
      os.stdio.println(r.to_hex())
    }
  }
}
`

// x509Seeds are the certificates of the TLS goldens.
func x509Seeds(t *testing.T) [][]byte {
	var out [][]byte
	re := regexp.MustCompile(`(?m)^const [A-Z0-9_]+: Str = "(30[0-9a-f]+)"$`)
	for _, g := range []string{"tls_x509", "tls_verify", "tls13_go", "tls13_rfc8448"} {
		src, err := os.ReadFile(filepath.Join("testdata", "golden", g, "main.xo"))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllStringSubmatch(string(src), -1) {
			b, _ := hex.DecodeString(m[1])
			// whole certificates only: a SEQUENCE spanning the constant
			if len(b) > 4 && b[1] == 0x82 && int(b[2])<<8|int(b[3]) == len(b)-4 {
				out = append(out, b)
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("no certificates")
	}
	return out
}

// derNode is a DER element: its tag, its contents (when primitive), or
// its children.
type derNode struct {
	tag      byte
	content  []byte
	kids     []*derNode
	compound bool
}

func derParse(b []byte) ([]*derNode, bool) {
	var out []*derNode
	for len(b) > 0 {
		if len(b) < 2 {
			return nil, false
		}
		tag, n, off := b[0], int(b[1]), 2
		if n&0x80 != 0 {
			k := n & 0x7f
			if k == 0 || k > 3 || len(b) < 2+k {
				return nil, false
			}
			n = 0
			for i := 0; i < k; i++ {
				n = n<<8 | int(b[2+i])
			}
			off = 2 + k
		}
		if len(b) < off+n {
			return nil, false
		}
		node := &derNode{tag: tag, content: b[off : off+n]}
		if tag&0x20 != 0 {
			if kids, ok := derParse(node.content); ok {
				node.kids, node.compound = kids, true
			}
		}
		out = append(out, node)
		b = b[off+n:]
	}
	return out, true
}

func derEncode(nodes []*derNode) []byte {
	var out []byte
	for _, n := range nodes {
		c := n.content
		if n.compound {
			c = derEncode(n.kids)
		}
		out = append(out, n.tag)
		switch l := len(c); {
		case l < 128:
			out = append(out, byte(l))
		case l < 256:
			out = append(out, 0x81, byte(l))
		default:
			out = append(out, 0x82, byte(l>>8), byte(l))
		}
		out = append(out, c...)
	}
	return out
}

func derAll(nodes []*derNode, out *[]*derNode) {
	for _, n := range nodes {
		*out = append(*out, n)
		if n.compound {
			derAll(n.kids, out)
		}
	}
}

// derFind is the first element of the tree (depth first) that ok picks.
func derFind(nodes []*derNode, ok func(*derNode) bool) *derNode {
	var all []*derNode
	derAll(nodes, &all)
	for _, n := range all {
		if ok(n) {
			return n
		}
	}
	return nil
}

// derExtension is the tree of the value of the extension with the OID
// contents oid (its OCTET STRING parsed) and a function that writes it
// back.
func derExtension(tree []*derNode, oid []byte) ([]*derNode, func()) {
	var all []*derNode
	derAll(tree, &all)
	for _, n := range all {
		if !n.compound || len(n.kids) < 2 || n.kids[0].tag != 6 || !bytes.Equal(n.kids[0].content, oid) {
			continue
		}
		val := n.kids[len(n.kids)-1]
		kids, ok := derParse(val.content)
		if !ok {
			return nil, nil
		}
		return kids, func() { val.content = derEncode(kids) }
	}
	return nil, nil
}

// x509Classes are one case for each kind of input Go's parser refuses
// (or takes) that x509.xo once handled differently: each a seed with one
// element changed (seeds without the element give no case).
func x509Classes(seeds [][]byte) map[string][]byte {
	out := map[string][]byte{}
	edit := func(name string, f func(tree []*derNode) bool) {
		for _, seed := range seeds {
			tree, ok := derParse(append([]byte(nil), seed...))
			if ok && f(tree) {
				out[name] = derEncode(tree)
				return
			}
		}
	}
	tagged := func(tag byte) func(*derNode) bool { return func(n *derNode) bool { return n.tag == tag } }
	str := func(n *derNode) bool { return n.tag == 0x13 || n.tag == 0x0c }
	edit("oid leading 0x80", func(t []*derNode) bool {
		n := derFind(t, tagged(6))
		n.content = append([]byte{0x80}, n.content...)
		return true
	})
	edit("oid arc of 2^31", func(t []*derNode) bool {
		n := derFind(t, tagged(6))
		n.content = append(append([]byte(nil), n.content...), 0x88, 0x80, 0x80, 0x80, 0x00)
		return true
	})
	edit("oid cut", func(t []*derNode) bool {
		n := derFind(t, tagged(6))
		n.content = append(append([]byte(nil), n.content...), 0x81)
		return true
	})
	for _, c := range []struct {
		name string
		tag  byte
		val  []byte
	}{
		{"printable @", 0x13, []byte("a@b")},
		{"printable * and &", 0x13, []byte("*.a&b")},
		{"utf8 invalid", 0x0c, []byte{0x61, 0xff}},
		{"utf8 surrogate", 0x0c, []byte{0xed, 0xa0, 0x80}},
		{"bmp odd", 0x1e, []byte{0, 0x61, 0}},
		{"bmp surrogate", 0x1e, []byte{0xd8, 0x00}},
		{"bmp terminator", 0x1e, []byte{0, 0x61, 0, 0}},
		{"bmp noncharacter", 0x1e, []byte{0xff, 0xfe}},
		{"t61 latin-1", 0x14, []byte{0x61, 0xe9}},
		{"ia5 non-ascii", 0x16, []byte{0x61, 0xe9}},
		{"numeric letter", 0x12, []byte("12a")},
		{"unsupported string type", 0x15, []byte("a")},
	} {
		edit(c.name, func(t []*derNode) bool {
			n := derFind(t, str)
			n.tag, n.content = c.tag, c.val
			return true
		})
	}
	edit("negative serial", func(t []*derNode) bool {
		n := derFind(t, tagged(2))
		n.content = append([]byte{0x80}, n.content...)
		return true
	})
	edit("non-minimal serial", func(t []*derNode) bool {
		n := derFind(t, tagged(2))
		n.content = append([]byte{0x00, 0x01}, n.content...)
		return true
	})
	for _, c := range []struct{ name, val string }{
		{"utc february 30", "250230000000Z"},
		{"utc +0000", "250101000000+0000"},
		{"utc offset", "250101000000-0130"},
		{"utc without seconds", "2501010000Z"},
		{"utc hour 24", "250101240000Z"},
		{"utc leap second", "250101235960Z"},
		{"utc year 50", "500101000000Z"},
	} {
		edit(c.name, func(t []*derNode) bool {
			n := derFind(t, tagged(0x17))
			if n == nil {
				return false
			}
			n.content = []byte(c.val)
			return true
		})
	}
	for _, c := range []struct{ name, val string }{
		{"generalized fraction", "20250101000000.5Z"},
		{"generalized", "20250101000000Z"},
		{"generalized leap day", "20240229000000Z"},
		{"generalized not leap", "21000229000000Z"},
	} {
		edit(c.name, func(t []*derNode) bool {
			n := derFind(t, tagged(0x17))
			if n == nil {
				return false
			}
			n.tag, n.content = 0x18, []byte(c.val)
			return true
		})
	}
	edit("point off the curve", func(t []*derNode) bool {
		n := derFind(t, func(n *derNode) bool { return n.tag == 3 && len(n.content) == 66 && n.content[1] == 4 })
		if n == nil {
			return false
		}
		n.content = append([]byte(nil), n.content...)
		n.content[65] ^= 1
		return true
	})
	edit("point coordinate not reduced", func(t []*derNode) bool {
		n := derFind(t, func(n *derNode) bool { return n.tag == 3 && len(n.content) == 66 && n.content[1] == 4 })
		if n == nil {
			return false
		}
		n.content = append([]byte(nil), n.content...)
		for i := 2; i < 34; i++ {
			n.content[i] = 0xff
		}
		return true
	})
	edit("rsa without null", func(t []*derNode) bool {
		alg := derFind(t, func(n *derNode) bool {
			return n.compound && len(n.kids) == 2 && n.kids[0].tag == 6 && bytes.Equal(n.kids[0].content, []byte{0x2a, 0x86, 0x48, 0x86, 0xf7, 0x0d, 0x01, 0x01, 0x01})
		})
		if alg == nil {
			return false
		}
		alg.kids = alg.kids[:1]
		return true
	})
	edit("bit string padding", func(t []*derNode) bool {
		n := derFind(t, func(n *derNode) bool { return n.tag == 3 && len(n.content) > 1 && n.content[len(n.content)-1]&1 == 1 })
		if n == nil {
			return false
		}
		n.content = append([]byte{7}, n.content[1:]...)
		return true
	})
	edit("high tag number", func(t []*derNode) bool {
		n := derFind(t, tagged(0xa3))
		if n == nil {
			return false
		}
		n.tag = 0xbf
		return true
	})
	edit("duplicate extension", func(t []*derNode) bool {
		n := derFind(t, tagged(0xa3))
		if n == nil || len(n.kids) == 0 || len(n.kids[0].kids) == 0 {
			return false
		}
		exts := n.kids[0]
		exts.kids = append(exts.kids, exts.kids[0])
		return true
	})
	edit("critical 0x01", func(t []*derNode) bool {
		n := derFind(t, tagged(0xa3))
		if n == nil || len(n.kids) == 0 {
			return false
		}
		b := derFind(n.kids, tagged(1))
		if b == nil {
			return false
		}
		b.content = []byte{1}
		return true
	})
	// the trailing byte goes on after the encoding
	edit("trailing data", func(t []*derNode) bool { return true })
	if b, ok := out["trailing data"]; ok {
		out["trailing data"] = append(b, 0)
	}
	edit("trailing in tbs", func(t []*derNode) bool {
		tbs := t[0].kids[0]
		tbs.kids = append(tbs.kids, &derNode{tag: 5})
		return true
	})
	edit("non-minimal length", func(t []*derNode) bool { return true })
	if b, ok := out["non-minimal length"]; ok && b[1] == 0x82 {
		out["non-minimal length"] = append([]byte{0x30, 0x83, 0x00}, b[2:]...)
	}
	san := []byte{0x55, 0x1d, 0x11}
	for _, c := range []struct {
		name string
		tag  byte
		val  []byte
	}{
		{"san dns non-ascii", 0x82, []byte{0x61, 0xe9}},
		{"san ip of 5 bytes", 0x87, []byte{1, 2, 3, 4, 5}},
		{"san email non-ascii", 0x81, []byte{0x61, 0xe9}},
		{"san uri non-ascii", 0x86, []byte{0x61, 0xe9}},
		{"san uri", 0x86, []byte("https://example.com/a?b#c")},
		{"san uri bad escape", 0x86, []byte("https://example.com/%zz")},
		{"san uri bad fragment", 0x86, []byte("https://example.com/#%zz")},
		{"san uri space in host", 0x86, []byte("https://exa mple.com/")},
		{"san uri bad port", 0x86, []byte("https://example.com:8x/")},
		{"san uri no scheme", 0x86, []byte("://example.com")},
		{"san uri empty label", 0x86, []byte("https://a..example.com/")},
		{"san uri trailing dot", 0x86, []byte("https://example.com./")},
		{"san uri ipv6", 0x86, []byte("https://[::1]:443/")},
		{"san uri bad ipv6", 0x86, []byte("https://[::1/")},
		{"san uri control", 0x86, []byte("https://example.com/\x01")},
		{"san uri colon in path", 0x86, []byte("a:b:c")},
	} {
		edit(c.name, func(t []*derNode) bool {
			v, done := derExtension(t, san)
			if v == nil || len(v) == 0 || len(v[0].kids) == 0 {
				return false
			}
			v[0].kids[0] = &derNode{tag: c.tag, content: c.val}
			done()
			return true
		})
	}
	nc := []byte{0x55, 0x1d, 0x1e}
	for _, c := range []struct {
		name string
		tag  byte
		val  []byte
	}{
		{"constraint ip bad mask", 0x87, []byte{10, 0, 0, 0, 255, 0, 255, 0}},
		{"constraint ip length", 0x87, []byte{10, 0, 0, 0, 255}},
		{"constraint dns trailing dot", 0x82, []byte("example.com.")},
		{"constraint dns empty label", 0x82, []byte("a..com")},
		{"constraint email", 0x81, []byte("a..b@example.com")},
		{"constraint uri ip", 0x86, []byte("10.0.0.1")},
		{"constraint other name", 0xa0, []byte{}},
	} {
		edit(c.name, func(t []*derNode) bool {
			v, done := derExtension(t, nc)
			if v == nil || len(v) == 0 || len(v[0].kids) == 0 || len(v[0].kids[0].kids) == 0 {
				return false
			}
			sub := v[0].kids[0].kids[0]
			sub.kids = []*derNode{{tag: c.tag, content: c.val, compound: c.tag&0x20 != 0}}
			done()
			return true
		})
	}
	return out
}

// x509Mutate changes a certificate: raw bytes, or one element of its
// tree with the lengths around it encoded again (so the change reaches
// the parser's deeper checks: strings, OIDs, times, integers,
// extensions).
func x509Mutate(rng *rand.Rand, seed []byte) []byte {
	b := append([]byte(nil), seed...)
	if rng.Intn(4) == 0 {
		switch rng.Intn(3) {
		case 0:
			b[rng.Intn(len(b))] ^= byte(1 + rng.Intn(255))
		case 1:
			b = b[:rng.Intn(len(b))]
		case 2:
			i := rng.Intn(len(b))
			b = append(b[:i:i], append([]byte{byte(rng.Intn(256))}, b[i:]...)...)
		}
		return b
	}
	tree, ok := derParse(b)
	if !ok {
		return b
	}
	var all []*derNode
	derAll(tree, &all)
	n := all[rng.Intn(len(all))]
	strings := []byte{0x0c, 0x13, 0x14, 0x16, 0x1e, 0x1c, 0x12, 0x1a}
	switch k := rng.Intn(9); {
	case k == 0:
		n.tag = strings[rng.Intn(len(strings))]
	case k == 1:
		n.tag ^= byte(1 << rng.Intn(8))
	case k == 2 && n.compound && len(n.kids) > 0:
		i := rng.Intn(len(n.kids))
		n.kids = append(n.kids[:i:i], n.kids[i+1:]...)
	case k == 3 && n.compound && len(n.kids) > 0:
		i := rng.Intn(len(n.kids))
		n.kids = append(n.kids[:i+1:i+1], n.kids[i:]...)
	case k == 4 && n.compound:
		n.kids = append(n.kids, &derNode{tag: byte(rng.Intn(256)) &^ 0x20, content: []byte{byte(rng.Intn(256))}})
	case !n.compound && len(n.content) > 0 && k < 7:
		c := append([]byte(nil), n.content...)
		switch rng.Intn(4) {
		case 0:
			c[rng.Intn(len(c))] ^= byte(1 + rng.Intn(255))
		case 1:
			c = c[:rng.Intn(len(c))]
		case 2:
			i := rng.Intn(len(c) + 1)
			c = append(c[:i:i], append([]byte{byte(rng.Intn(256))}, c[i:]...)...)
		case 3:
			// a leading byte: a non-minimal integer or OID arc, padding
			c = append([]byte{[]byte{0x00, 0x80, 0xff}[rng.Intn(3)]}, c...)
		}
		n.content = c
	default:
		// a primitive becomes empty, or a compound one primitive
		n.compound, n.kids, n.content = false, nil, nil
	}
	return derEncode(tree)
}

// TestX509ParseDiff parses mutated certificates with x509.xo (LLVM and
// arm64) and Go's x509.ParseCertificate (the Go backend) and diffs the
// outcome and its text (decision 0202: x509.xo refuses at least what Go
// refuses, with Go's error). XO_X509_DIFF_N cases (default 5000) from
// XO_X509_DIFF_SEED (default 7).
func TestX509ParseDiff(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program with three backends")
	}
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(x509DiffProgram), 0o644); err != nil {
		t.Fatal(err)
	}
	mod := "module github.com/xo-lang/xo/test/x509diff\nedition preview\n"
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	n := 5000
	if v, err := strconv.Atoi(os.Getenv("XO_X509_DIFF_N")); err == nil {
		n = v
	}
	seed := int64(7)
	if v, err := strconv.ParseInt(os.Getenv("XO_X509_DIFF_SEED"), 10, 64); err == nil {
		seed = v
	}
	seeds := x509Seeds(t)
	rng := rand.New(rand.NewSource(seed))
	var in strings.Builder
	var cases [][]byte
	var names []string
	for _, s := range seeds {
		cases = append(cases, s)
		names = append(names, "seed")
	}
	classes0 := x509Classes(seeds)
	var classNames []string
	for k := range classes0 {
		classNames = append(classNames, k)
	}
	sort.Strings(classNames)
	for _, k := range classNames {
		cases = append(cases, classes0[k])
		names = append(names, k)
	}
	t.Logf("%d classes: %s", len(classNames), strings.Join(classNames, ", "))
	for len(cases) < n {
		m := x509Mutate(rng, seeds[rng.Intn(len(seeds))])
		if len(m) > 0 {
			cases = append(cases, m)
			names = append(names, "mutation")
		}
	}
	for _, c := range cases {
		in.WriteString(hex.EncodeToString(c) + "\n")
	}
	backends := []string{xotool.BackendGo, xotool.BackendLLVM}
	if runtime.GOARCH == "arm64" {
		backends = append(backends, xotool.BackendArm64)
	}
	outs := make([][]string, len(backends))
	for i, be := range backends {
		res, err := xotool.Build([]string{dir}, xotool.Options{Backend: be})
		if err != nil {
			t.Fatalf("build (%s): %v", be, err)
		}
		if res.Failed {
			t.Fatalf("build (%s) failed: %v", be, res.Output)
		}
		cmd := testtmp.Guard(exec.Command(res.Binaries[xotool.Host()]))
		cmd.Stdin = strings.NewReader(in.String())
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%s: %v\n%s", be, err, stderr.String())
		}
		outs[i] = strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
		if len(outs[i]) != len(cases) {
			t.Fatalf("%s: %d results for %d cases", be, len(outs[i]), len(cases))
		}
	}
	// the classes of differences: what Go says against what x509.xo says
	classes := map[string]int{}
	examples := map[string]int{}
	accepted := 0
	for i := range cases {
		ref, _ := hex.DecodeString(outs[0][i])
		if !strings.HasPrefix(string(ref), "error ") {
			accepted++
		}
		for b := 1; b < len(backends); b++ {
			if outs[b][i] == outs[0][i] {
				continue
			}
			got, _ := hex.DecodeString(outs[b][i])
			k := x509DiffKind(string(ref)) + "  <>  " + x509DiffKind(string(got))
			if names[i] != "mutation" {
				k = names[i] + ": " + k
			}
			if classes[k] == 0 {
				examples[k] = i
			}
			classes[k]++
		}
	}
	var keys []string
	for k := range classes {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return classes[keys[i]] > classes[keys[j]] })
	bad := 0
	for _, k := range keys {
		bad += classes[k]
		t.Errorf("%5d  %s  (case %d: %s)", classes[k], k, examples[k], hex.EncodeToString(cases[examples[k]]))
	}
	t.Logf("%d cases, %d that Go accepts, %d differences", len(cases), accepted, bad)
}

// x509DiffKind is an outcome without the description's fields.
func x509DiffKind(out string) string {
	if strings.HasPrefix(out, "error ") {
		return out
	}
	return "ok"
}
