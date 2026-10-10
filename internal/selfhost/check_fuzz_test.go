package selfhost

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/lexer"
	"github.com/xo-lang/xo/internal/token"
)

// TestSelfhostCheckerNamesMutations compares the Xo and Go checkers
// (decision 0141) on mutations of the single directory modules of the
// repository that the Go checker accepts and that import only std: one
// identifier renamed (an unresolved name, or one that now resolves
// elsewhere), a `use` line removed, or a top level declaration
// duplicated. The rules of TestSelfhostCheckerNames apply: an accepted
// mutation has the same names section, one rejected only for names the
// same diagnostics. -short makes 150 mutations, else 1000;
// XO_SELFHOST_SEED and XO_SELFHOST_CHECK_FUZZ=N override.
func TestSelfhostCheckerNamesMutations(t *testing.T) {
	n := 1000
	if testing.Short() {
		n = 150
	}
	if v := os.Getenv("XO_SELFHOST_CHECK_FUZZ"); v != "" {
		fmt.Sscan(v, &n)
	}
	seed := int64(20261010)
	if v := os.Getenv("XO_SELFHOST_SEED"); v != "" {
		fmt.Sscan(v, &seed)
	}
	root := repoRoot(t)
	bin, err := selfhostBinary("check", "go")
	if err != nil {
		t.Fatal(err)
	}
	srcs := mutationSources(t, root)
	if len(srcs) < 50 {
		t.Fatalf("only %d source modules", len(srcs))
	}
	tmp := t.TempDir()
	if keep := os.Getenv("XO_SELFHOST_KEEP"); keep != "" {
		tmp = filepath.Join(keep, "check-mutations")
		if err := os.MkdirAll(tmp, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	stdDir, err := filepath.Abs(filepath.Join(root, "std"))
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(seed))
	var dirs []string
	for i := range n {
		src := srcs[rng.Intn(len(srcs))]
		dir := fmt.Sprintf("m%04d", i)
		if err := os.MkdirAll(filepath.Join(tmp, dir), 0o755); err != nil {
			t.Fatal(err)
		}
		f := rng.Intn(len(src.files))
		for j, name := range src.names {
			data := src.files[j]
			if j == f {
				data = mutateNames(rng, data)
			}
			if err := os.WriteFile(filepath.Join(tmp, dir, name), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		dirs = append(dirs, dir)
	}
	t.Chdir(tmp)
	got := map[string]string{}
	const batch = 100
	for i := 0; i < len(dirs); i += batch {
		chunk := dirs[i:min(i+batch, len(dirs))]
		text := runXo(t, bin, tmp, append([]string{"--each", "--std=" + stdDir}, chunk...))
		parts := strings.Split(text, "=== check ")
		if len(parts) != len(chunk)+1 {
			t.Fatalf("Xo checker printed %d runs for %d directories", len(parts)-1, len(chunk))
		}
		for j, d := range chunk {
			got[d] = strings.TrimPrefix(parts[j+1], d+"\n")
		}
	}
	var st checkStats
	var fails []string
	for _, d := range dirs {
		if msg := compareRun(d, goCheck(d), got[d], &st); msg != "" {
			src, _ := os.ReadFile(filepath.Join(tmp, d, firstXo(t, filepath.Join(tmp, d))))
			fails = append(fails, msg+"\n"+firstLines(string(src), 0))
		}
	}
	t.Logf("%d mutations: %d accepted by Go (%d match), %d rejected only for names (%d match), %d rejected otherwise, %d unsupported",
		len(dirs), st.accepted, st.acceptedOK, st.nameRejected, st.nameRejectedOK, st.otherRejected, st.unsupported)
	if len(fails) > 0 {
		if len(fails) > 5 {
			fails = append(fails[:5], fmt.Sprintf("... and %d more", len(fails)-5))
		}
		t.Fatalf("%d of %d mutations differ:\n%s", len(fails), len(dirs), strings.Join(fails, "\n"))
	}
	if st.nameRejectedOK < len(dirs)/10 {
		t.Fatalf("only %d of %d mutations were rejected only for names", st.nameRejectedOK, len(dirs))
	}
}

func firstXo(t *testing.T, dir string) string {
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range es {
		if strings.HasSuffix(e.Name(), ".xo") {
			return e.Name()
		}
	}
	return ""
}

func firstLines(s string, n int) string {
	if n <= 0 {
		return ""
	}
	ls := strings.SplitN(s, "\n", n+1)
	return strings.Join(ls[:min(n, len(ls))], "\n")
}

// mutationSource is a module of one directory: its file names and
// contents.
type mutationSource struct {
	names []string
	files [][]byte
}

var stdUseRE = regexp.MustCompile(`(?m)^use\s+(\S+)`)

// mutationSources are the single directory modules the Go checker
// accepts that import only std modules and have no xo.mod (their copies
// check the same).
func mutationSources(t *testing.T, root string) []mutationSource {
	var out []mutationSource
	for _, d := range moduleDirs(t, root) {
		full := filepath.Join(root, d)
		es, err := os.ReadDir(full)
		if err != nil {
			t.Fatal(err)
		}
		var src mutationSource
		ok := true
		for _, e := range es {
			if e.IsDir() || e.Name() == "xo.mod" {
				ok = false
				break
			}
			if !strings.HasSuffix(e.Name(), ".xo") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(full, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range stdUseRE.FindAllSubmatch(data, -1) {
				if !bytes.HasPrefix(m[1], []byte("std/")) {
					ok = false
				}
			}
			src.names = append(src.names, e.Name())
			src.files = append(src.files, data)
		}
		if !ok || len(src.files) == 0 || hasModAbove(root, full) {
			continue
		}
		res := goCheckAt(root, d)
		if res == nil || !res.accepted {
			continue
		}
		out = append(out, src)
	}
	return out
}

// goCheckAt checks dir (relative to root) from root.
func goCheckAt(root, dir string) *goRun {
	wd, err := os.Getwd()
	if err != nil {
		return nil
	}
	if err := os.Chdir(root); err != nil {
		return nil
	}
	defer os.Chdir(wd)
	r := goCheck(dir)
	if r.err != nil {
		return nil
	}
	return r
}

// hasModAbove reports whether an xo.mod lies in dir or a parent below
// root.
func hasModAbove(root, dir string) bool {
	for d := dir; len(d) >= len(root); d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "xo.mod")); err == nil {
			return true
		}
		if d == root {
			break
		}
	}
	return false
}

// mutateNames applies one name level edit to src: rename an identifier
// (append a letter, or take another identifier's name), remove a `use`
// line, or duplicate a top level declaration line.
func mutateNames(rng *rand.Rand, src []byte) []byte {
	var ds diag.List
	toks := lexer.Lex("f.xo", src, &ds).Tokens
	var ids []token.Token
	for _, tk := range toks {
		if tk.Kind == token.Ident {
			ids = append(ids, tk)
		}
	}
	lines := bytes.Split(src, []byte("\n"))
	switch k := rng.Intn(10); {
	case k < 6 && len(ids) > 0:
		id := ids[rng.Intn(len(ids))]
		name := id.Text + "q"
		if rng.Intn(2) == 0 {
			name = ids[rng.Intn(len(ids))].Text
		}
		var b bytes.Buffer
		b.Write(src[:id.Span.Start.Offset])
		b.WriteString(name)
		b.Write(src[id.Span.End.Offset:])
		return b.Bytes()
	case k < 8:
		var uses []int
		for i, l := range lines {
			if bytes.HasPrefix(l, []byte("use ")) {
				uses = append(uses, i)
			}
		}
		if len(uses) > 0 {
			i := uses[rng.Intn(len(uses))]
			lines = append(lines[:i:i], lines[i+1:]...)
			return bytes.Join(lines, []byte("\n"))
		}
	}
	// duplicate a one line declaration, or the header of one
	var decls []int
	for i, l := range lines {
		for _, p := range []string{"fn ", "pub fn ", "struct ", "enum ", "const ", "type "} {
			if bytes.HasPrefix(l, []byte(p)) && bytes.Count(l, []byte("{")) == bytes.Count(l, []byte("}")) {
				decls = append(decls, i)
			}
		}
	}
	if len(decls) == 0 {
		return append(src, []byte("\nconst DUPLICATE_ME = 1\nconst DUPLICATE_ME = 2\n")...)
	}
	i := decls[rng.Intn(len(decls))]
	out := append([][]byte{}, lines[:i+1]...)
	out = append(out, lines[i])
	out = append(out, lines[i+1:]...)
	return bytes.Join(out, []byte("\n"))
}
