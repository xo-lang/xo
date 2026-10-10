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

// TestSelfhostCheckerMutations compares the Xo and Go checkers (decision
// 0141) on mutations of the single directory modules of the repository
// that the Go checker accepts and that import only std. A quarter each
// are name mutations (one identifier renamed, to an unresolved name or
// one that now resolves elsewhere, a `use` line removed, or a top level
// declaration duplicated), type mutations (a literal replaced by one of
// another type, a `?` removed, or a field or method name replaced by
// another one of the file), generics mutations (a type argument removed,
// a constraint dropped), and effects mutations (a `uses` clause removed
// or added). The rules of TestSelfhostChecker apply: an accepted mutation
// has the same names, types, and sigs sections, one rejected only with
// diagnostics of the stages done the same diagnostics. -short makes 150
// mutations, else 1000; XO_SELFHOST_SEED and XO_SELFHOST_CHECK_FUZZ=N
// override.
func TestSelfhostCheckerMutations(t *testing.T) {
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
				switch rng.Intn(4) {
				case 0:
					data = mutateNames(rng, data)
				case 1:
					data = mutateTypes(rng, data)
				case 2:
					data = mutateGenerics(rng, data)
				default:
					data = mutateEffects(rng, data)
				}
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
	codes := map[string]int{}
	for _, d := range dirs {
		want := goCheck(d)
		before := st.stagedOK
		if msg := compareRun(d, want, got[d], &st); msg != "" {
			src, _ := os.ReadFile(filepath.Join(tmp, d, firstXo(t, filepath.Join(tmp, d))))
			fails = append(fails, msg+"\n"+firstLines(string(src), 0))
		}
		if st.stagedOK > before {
			for _, l := range want.staged {
				codes[l[len(`{"code":"`):len(`{"code":"XO0000`)]]++
			}
		}
	}
	t.Logf("%d mutations: %d accepted by Go (%d match), %d rejected only with diagnostics of the stages done (%d match), %d rejected otherwise, %d unsupported",
		len(dirs), st.accepted, st.acceptedOK, st.staged, st.stagedOK, st.otherRejected, st.unsupported)
	t.Logf("diagnostics of the matching rejected mutations: %v", codes)
	if len(fails) > 0 {
		if len(fails) > 5 {
			fails = append(fails[:5], fmt.Sprintf("... and %d more", len(fails)-5))
		}
		t.Fatalf("%d of %d mutations differ:\n%s", len(fails), len(dirs), strings.Join(fails, "\n"))
	}
	if st.stagedOK < len(dirs)/10 {
		t.Fatalf("only %d of %d mutations were rejected only with diagnostics of the stages done", st.stagedOK, len(dirs))
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

// mutateTypes applies one type level edit to src: an integer literal
// becomes a string, float, or Bool literal, a string literal an integer,
// a postfix `?` is removed, or the name after a `.` becomes another
// such name of the file (another field or method).
func mutateTypes(rng *rand.Rand, src []byte) []byte {
	var ds diag.List
	toks := lexer.Lex("f.xo", src, &ds).Tokens
	var lits, strs, quests, sels []token.Token
	for i, tk := range toks {
		switch tk.Kind {
		case token.Int:
			lits = append(lits, tk)
		case token.String:
			strs = append(strs, tk)
		case token.Question:
			if i > 0 && toks[i-1].Span.End.Offset == tk.Span.Start.Offset {
				quests = append(quests, tk)
			}
		case token.Ident:
			if i > 0 && toks[i-1].Kind == token.Dot {
				sels = append(sels, tk)
			}
		}
	}
	replace := func(tk token.Token, with string) []byte {
		var b bytes.Buffer
		b.Write(src[:tk.Span.Start.Offset])
		b.WriteString(with)
		b.Write(src[tk.Span.End.Offset:])
		return b.Bytes()
	}
	for range 4 {
		switch rng.Intn(4) {
		case 0:
			if len(lits) > 0 {
				return replace(lits[rng.Intn(len(lits))], []string{`"s"`, "1.5", "true", "'a'"}[rng.Intn(4)])
			}
		case 1:
			if len(strs) > 0 {
				return replace(strs[rng.Intn(len(strs))], "0")
			}
		case 2:
			if len(quests) > 0 {
				return replace(quests[rng.Intn(len(quests))], "")
			}
		case 3:
			if len(sels) > 1 {
				return replace(sels[rng.Intn(len(sels))], sels[rng.Intn(len(sels))].Text)
			}
		}
	}
	return mutateNames(rng, src)
}

// mutateGenerics applies one generics edit to src: a type argument
// removed from a written list (`Map[Str, Int]` becomes `Map[Str]`,
// `parse[Int](s)` becomes `parse(s)`), or a constraint dropped from a
// type parameter (`[T: Ord]` becomes `[T]`).
func mutateGenerics(rng *rand.Rand, src []byte) []byte {
	var ds diag.List
	toks := lexer.Lex("f.xo", src, &ds).Tokens
	upper := func(tk token.Token) bool {
		return tk.Kind == token.Ident && tk.Text != "" && tk.Text[0] >= 'A' && tk.Text[0] <= 'Z'
	}
	// closing finds the token ending the element that starts at i of a
	// bracketed list: a Comma or the RBrack at depth 0.
	closing := func(i int) int {
		depth := 0
		for ; i < len(toks); i++ {
			switch toks[i].Kind {
			case token.LBrack, token.LParen, token.LBrace:
				depth++
			case token.RParen, token.RBrace:
				depth--
			case token.RBrack:
				if depth == 0 {
					return i
				}
				depth--
			case token.Comma:
				if depth == 0 {
					return i
				}
			case token.Newline, token.EOF:
				return -1
			}
		}
		return -1
	}
	type edit struct{ from, to int }
	var args, constraints []edit
	for i := 1; i+1 < len(toks); i++ {
		tk := toks[i]
		switch {
		case tk.Kind == token.LBrack && toks[i-1].Kind == token.Ident && toks[i-1].Span.End.Offset == tk.Span.Start.Offset && upper(toks[i+1]):
			// the arguments of a type or call: remove one, or the list
			var elems []int // start token of each element
			j := i + 1
			for {
				elems = append(elems, j)
				k := closing(j)
				if k < 0 {
					elems = nil
					break
				}
				if toks[k].Kind == token.RBrack {
					if len(elems) == 1 {
						args = append(args, edit{tk.Span.Start.Offset, toks[k].Span.End.Offset})
						elems = nil
					} else {
						elems = append(elems, k)
					}
					break
				}
				j = k + 1
			}
			for e := 0; e+1 < len(elems); e++ {
				// element e runs from elems[e] to the token before elems[e+1]
				// (a Comma, or the RBrack for the last); drop it with one comma
				from := toks[elems[e]].Span.Start.Offset
				to := toks[elems[e+1]].Span.Start.Offset
				if e == len(elems)-2 {
					from = toks[elems[e]-1].Span.Start.Offset // the comma before the last
					to = toks[elems[e+1]].Span.Start.Offset
				}
				args = append(args, edit{from, to})
			}
		case tk.Kind == token.Colon && upper(toks[i-1]) && i >= 2 && (toks[i-2].Kind == token.LBrack || toks[i-2].Kind == token.Comma):
			if k := closing(i + 1); k >= 0 {
				constraints = append(constraints, edit{tk.Span.Start.Offset, toks[k].Span.Start.Offset})
			}
		}
	}
	var pick []edit
	switch {
	case len(args) > 0 && (len(constraints) == 0 || rng.Intn(2) == 0):
		pick = args
	case len(constraints) > 0:
		pick = constraints
	default:
		return mutateTypes(rng, src)
	}
	e := pick[rng.Intn(len(pick))]
	var b bytes.Buffer
	b.Write(src[:e.from])
	b.Write(src[e.to:])
	return b.Bytes()
}

// mutateEffects applies one effects edit to src: a `uses` clause
// removed, or one added to a function declaration that has none.
func mutateEffects(rng *rand.Rand, src []byte) []byte {
	var ds diag.List
	toks := lexer.Lex("f.xo", src, &ds).Tokens
	type edit struct {
		from, to int
		with     string
	}
	var edits []edit
	for i := 1; i < len(toks); i++ {
		if toks[i].Kind != token.Uses {
			continue
		}
		j := i + 1
		depth := 0
	clause:
		for ; j < len(toks); j++ {
			switch toks[j].Kind {
			case token.Ident, token.Dot, token.Comma:
			case token.LParen:
				if j != i+1 {
					break clause
				}
				depth++
			case token.RParen:
				if depth == 0 {
					break clause
				}
				depth--
			default:
				break clause
			}
		}
		if j < len(toks) {
			edits = append(edits, edit{toks[i-1].Span.End.Offset, toks[j].Span.Start.Offset, ""})
		}
	}
	labels := []string{"net", "clock", "fs.read", "stdio", "rand", "env"}
	off := 0
	for _, l := range bytes.SplitAfter(src, []byte("\n")) {
		t := bytes.TrimRight(l, "\n")
		if (bytes.HasPrefix(t, []byte("fn ")) || bytes.HasPrefix(t, []byte("pub fn "))) && bytes.HasSuffix(t, []byte(" {")) && !bytes.Contains(t, []byte(" uses ")) {
			at := off + len(t) - 2
			edits = append(edits, edit{at, at, " uses " + labels[rng.Intn(len(labels))]})
		}
		off += len(l)
	}
	if len(edits) == 0 {
		return mutateTypes(rng, src)
	}
	e := edits[rng.Intn(len(edits))]
	var b bytes.Buffer
	b.Write(src[:e.from])
	b.WriteString(e.with)
	b.Write(src[e.to:])
	return b.Bytes()
}
