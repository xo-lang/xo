package main

import (
	"fmt"
	"math/rand/v2"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

// globTable is Go's path.Match test table (path/match_test.go), the
// cases whose pattern MemFs.fail_next keeps as written.
var globTable = [][2]string{
	{"abc", "abc"}, {"*", "abc"}, {"*c", "abc"}, {"a*", "a"}, {"a*", "abc"}, {"a*", "ab/c"},
	{"a*/b", "abc/b"}, {"a*/b", "a/c/b"}, {"a*b*c*d*e*/f", "axbxcxdxe/f"}, {"a*b*c*d*e*/f", "axbxcxdxexxx/f"},
	{"a*b*c*d*e*/f", "axbxcxdxe/xxx/f"}, {"a*b*c*d*e*/f", "axbxcxdxexxx/fff"}, {"a*b?c*x", "abxbbxdbxebxczzx"},
	{"a*b?c*x", "abxbbxdbxebxczzy"}, {"ab[c]", "abc"}, {"ab[b-d]", "abc"}, {"ab[e-g]", "abc"}, {"ab[^c]", "abc"},
	{"ab[^b-d]", "abc"}, {"ab[^e-g]", "abc"}, {"a\\*b", "a*b"}, {"a\\*b", "ab"}, {"a?b", "a☺b"}, {"a[^a]b", "a☺b"},
	{"a???b", "a☺b"}, {"a[^a][^a][^a]b", "a☺b"}, {"[a-ζ]*", "α"}, {"*[a-ζ]", "A"}, {"a?b", "a/b"}, {"a*b", "a/b"},
	{"[\\]a]", "]"}, {"[\\-]", "-"}, {"[x\\-]", "x"}, {"[x\\-]", "-"}, {"[x\\-]", "z"}, {"[\\-x]", "x"},
	{"[\\-x]", "-"}, {"[\\-x]", "a"}, {"[]a]", "]"}, {"[-]", "-"}, {"[x-]", "x"}, {"[x-]", "-"}, {"[x-]", "z"},
	{"[-x]", "x"}, {"[-x]", "-"}, {"[-x]", "a"}, {"\\", "a"}, {"[a-b-c]", "a"}, {"[", "a"}, {"[^", "a"},
	{"[^bc", "a"}, {"a[", "a"}, {"a[", "ab"}, {"a[", "x"}, {"a/b[", "x"}, {"*x", "xxx"},
}

// globCases are globTable and random patterns and names over the
// characters path.Match treats specially, each pattern as MemFs keeps
// it (path.Clean, no leading /). MemFs resolves a pattern without a `*`
// and every written name as a path, where `\\` is a separator: those
// keep no backslash.
func globCases() [][2]string {
	rng := rand.New(rand.NewPCG(11, 12))
	pchars := []string{"a", "b", "c", "*", "*", "?", "[", "]", "^", "-", "\\", "/", "é", "日"}
	nchars := []string{"a", "b", "c", "-", "]", "*", "é", "日", "^"}
	var out [][2]string
	keep := func(c [2]string) bool {
		return !strings.Contains(c[1], `\`) && (strings.Contains(c[0], "*") || !strings.Contains(c[0], `\`))
	}
	for _, c := range globTable {
		if keep(c) {
			out = append(out, c)
		}
	}
	for len(out) < 600 {
		var p, n strings.Builder
		for range 1 + rng.IntN(8) {
			p.WriteString(pchars[rng.IntN(len(pchars))])
		}
		for k := range 1 + rng.IntN(7) {
			if k > 0 && rng.IntN(5) == 0 {
				n.WriteString("/")
			}
			n.WriteString(nchars[rng.IntN(len(nchars))])
		}
		ps := p.String()
		ns := n.String()
		if rng.IntN(2) == 0 {
			// a name made from the pattern, so that more cases match
			var m strings.Builder
			for k := 0; k < len(ps); k++ {
				switch ps[k] {
				case '*':
					for range rng.IntN(3) {
						m.WriteString(nchars[rng.IntN(len(nchars))])
					}
				case '?', '[':
					m.WriteString(nchars[rng.IntN(len(nchars))])
				case '\\':
				default:
					m.WriteByte(ps[k])
				}
			}
			ns = m.String()
		}
		c := [2]string{ps, ns}
		if ns == "" || path.Clean(ns) != ns || strings.HasPrefix(ns, "/") {
			continue
		}
		if path.Clean(ps) != ps || strings.HasPrefix(ps, "/") || !keep(c) {
			continue
		}
		out = append(out, c)
	}
	return out
}

// xoQuote is s as an Xo string literal.
func xoQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// globTestFile is the test file of TestGlobDiff and the number of cases
// that match.
func globTestFile() (string, int) {
	var b strings.Builder
	b.WriteString("use std/testing\n")
	matched := 0
	for i, c := range globCases() {
		want, err := path.Match(c[0], c[1])
		if err != nil {
			want = false
		}
		if want {
			matched++
		}
		fmt.Fprintf(&b, "\ntest \"glob %d\" {\n  os.fs.fail_next(testing.FsOp.Write, %s, FsErr.Canceled)\n  expect(os.fs.write(%s, b\"\") == Err(FsErr.Canceled)) == %v\n}\n", i, xoQuote(c[0]), xoQuote(c[1]), want)
	}
	return b.String(), matched
}

// TestGlobDiff checks MemFs.fail_next's pattern match (path.Match, std/
// native/llrt/glob.xo, decision 0195) against Go's path.Match: one test
// per case expects the write to fail exactly when path.Match matches,
// on the Go backend and on both native backends (decision 0198 deleted
// the C path); every test passes and the reports are the same.
func TestGlobDiff(t *testing.T) {
	if testing.Short() {
		t.Skip("builds test binaries on every backend")
	}
	backends := nativeBackends(t)
	dir := t.TempDir()
	text, matched := globTestFile()
	t.Logf("%d cases, %d match", len(globCases()), matched)
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module example.com/glob\nedition preview\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "glob_test.xo"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XO_BUILD_DIR", filepath.Join(t.TempDir(), "build"))
	goCode, goOut := xoTest(t, "-v", dir)
	if goCode != 0 {
		t.Fatalf("go backend: exit %d\n%s", goCode, goOut)
	}
	for _, be := range backends {
		code, out := xoTest(t, "--no-gc", "--backend="+be, "-v", dir)
		if code != goCode || out != goOut {
			t.Errorf("%s differs from the Go backend (exit %d, %d)\n--- go\n%s--- %s\n%s", be, goCode, code, goOut, be, out)
		}
	}
}
