package selfhost

import (
	"bytes"
	"github.com/xo-lang/xo/internal/testtmp"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// defhashCorpus are the trees whose directories TestSelfhostDefhash
// hashes: every directory holding .xo files or an xo.mod is one module.
var defhashCorpus = []string{"bench/xo", "bench/paper", "internal/check/testdata", "internal/gen/testdata", "internal/arm64/testdata", "compiler", "std", "examples"}

// TestSelfhostDefhash compares the definition hashes of the Xo port
// (compiler/cli/semantic, `semantic defhash`) with the recording
// (internal/defhash's until decision 0215) on every module of the corpus,
// byte for byte as JSON: IDs, kinds, own and deep hashes, references,
// files, and lines (decision 0178 stage 3).
func TestSelfhostDefhash(t *testing.T) {
	skipDumpReplay(t)
	if testing.Short() {
		t.Skip("builds compiler/cli/semantic")
	}
	root := repoRoot(t)
	var args []string
	var mods []string
	for _, r := range defhashCorpus {
		top := filepath.Join(root, r)
		if _, err := os.Stat(top); err != nil {
			continue
		}
		err := filepath.WalkDir(top, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				return nil
			}
			if p != top && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			ents, err := os.ReadDir(p)
			if err != nil {
				return err
			}
			for _, e := range ents {
				if !e.IsDir() && (strings.HasSuffix(e.Name(), ".xo") || e.Name() == "xo.mod") {
					rel, _ := filepath.Rel(root, p)
					mod := "corpus.example/" + filepath.ToSlash(rel)
					// a module's own path, so imports of its other
					// packages resolve to their definitions
					if mp := xoModPath(p); mp != "" {
						mod = mp
					}
					args = append(args, p+"="+mod)
					mods = append(mods, p+"="+mod)
					break
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	sort.Strings(args)
	sort.Strings(mods)
	bin, err := selfhostBinary("cli/semantic", "go")
	if err != nil {
		t.Fatal(err)
	}
	cmd := testtmp.Guard(exec.Command(bin, append([]string{"defhash"}, args...)...))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("semantic defhash: %v\n%s", err, stderr.String())
	}
	got := splitDefhashDumps(string(out))
	same, defs := 0, 0
	for _, m := range mods {
		dir, path, _ := strings.Cut(m, "=")
		g, ok := got[m]
		gp := goDefhash(t, dir, path, g)
		// replayed, the recorded side's text is known when the hashes agree
		want, known := gp.resolve("-", g)
		if known {
			defs += defhashCount(want)
		}
		switch {
		case gp.missing:
			t.Error(gp.missingText())
		case !ok:
			t.Errorf("%s: no output from the Xo port", path)
		case !known:
			t.Errorf("%s: the Xo port differs from internal/defhash: %s", path, gp.diff("-", g))
		case g != want:
			t.Errorf("%s: the Xo port differs from internal/defhash: %s", path, firstDifference(want, g))
		default:
			same++
		}
	}
	t.Logf("%d of %d modules (%d definitions) byte identical", same, len(mods), defs)
}

// goDefhash is the recorded definition hash dump of module path in dir,
// as the Xo port prints it (the defhash kind, gogoldens_test.go: the
// whole dump is piece "-"; the module's files are the key); x is the Xo
// port's, which a recording run records.
func goDefhash(t *testing.T, dir, path, x string) *goldPieces {
	t.Helper()
	cx := newGoldenCtx(dir)
	key := []string{path, cx.norm(dir), treeSum(dir)}
	hint := goldHint("TestSelfhostDefhash", "cli/semantic", "defhash "+dir+"="+path, repoRoot(t))
	return recordedPieces("defhash", dir+" "+path, hint, key, cx, func() *goldPieces {
		g := newPieces()
		g.add("-", x)
		return g
	})
}

// xoModPath is the module path the xo.mod in dir declares, or "".
func xoModPath(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "xo.mod"))
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(string(data), "\n") {
		if f := strings.Fields(l); len(f) == 2 && f[0] == "module" {
			return strings.Trim(f[1], `"`)
		}
	}
	return ""
}

// defhashCount is the number of definitions of a dump of goDefhash.
func defhashCount(out string) int {
	if strings.HasPrefix(out, "error ") {
		return 0
	}
	return strings.Count(out, "\n  {")
}

// splitDefhashDumps splits `semantic defhash` output into module path -> dump.
func splitDefhashDumps(out string) map[string]string {
	res := map[string]string{}
	cur := ""
	var b strings.Builder
	flush := func() {
		if cur != "" {
			res[cur] = b.String()
		}
		b.Reset()
	}
	for _, line := range strings.SplitAfter(out, "\n") {
		if name, ok := strings.CutPrefix(line, "=== "); ok {
			flush()
			cur = strings.TrimSuffix(name, "\n")
			continue
		}
		b.WriteString(line)
	}
	flush()
	return res
}

// firstDifference names the first line where a and b differ.
func firstDifference(a, b string) string {
	al := strings.Split(a, "\n")
	bl := strings.Split(b, "\n")
	for i := 0; i < len(al) || i < len(bl); i++ {
		var x, y string
		if i < len(al) {
			x = al[i]
		}
		if i < len(bl) {
			y = bl[i]
		}
		if x != y {
			return "line " + strconv.Itoa(i+1) + ": want " + x + ", got " + y
		}
	}
	return "equal"
}
