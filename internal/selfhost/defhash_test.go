package selfhost

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/defhash"
	modpkg "github.com/xo-lang/xo/internal/mod"
)

// defhashCorpus are the trees whose directories TestSelfhostDefhash
// hashes: every directory holding .xo files or an xo.mod is one module.
var defhashCorpus = []string{"bench/xo", "bench/paper", "internal/check/testdata", "internal/gen/testdata", "internal/arm64/testdata", "selfhost", "std", "examples"}

// TestSelfhostDefhash compares the definition hashes of the Xo port
// (selfhost/cli/semantic, `semantic defhash`) with internal/defhash on
// every module of the corpus, byte for byte as JSON: IDs, kinds, own and
// deep hashes, references, files, and lines (decision 0178 stage 3).
func TestSelfhostDefhash(t *testing.T) {
	if testing.Short() {
		t.Skip("builds selfhost/cli/semantic")
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
					if f, _ := modpkg.ReadFile(p); f != nil && f.Path != "" {
						mod = f.Path
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
	cmd := exec.Command(bin, append([]string{"defhash"}, args...)...)
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
		ds, err := defhash.Module(dir, path)
		var want string
		if err != nil {
			want = "error " + err.Error() + "\n"
		} else {
			if ds == nil {
				ds = []defhash.Def{}
			}
			var b bytes.Buffer
			enc := json.NewEncoder(&b)
			enc.SetEscapeHTML(false)
			enc.SetIndent("", "  ")
			if err := enc.Encode(ds); err != nil {
				t.Fatal(err)
			}
			want = b.String()
			defs += len(ds)
		}
		g, ok := got[m]
		switch {
		case !ok:
			t.Errorf("%s: no output from the Xo port", path)
		case g != want:
			t.Errorf("%s: the Xo port differs from internal/defhash: %s", path, firstDifference(want, g))
		default:
			same++
		}
	}
	t.Logf("%d of %d modules (%d definitions) byte identical", same, len(mods), defs)
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
