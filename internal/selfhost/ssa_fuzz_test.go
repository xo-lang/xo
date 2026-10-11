package selfhost

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/lexer"
	"github.com/xo-lang/xo/internal/token"
)

// TestSelfhostSSAMutations compares the Xo and Go SSA builders (decision
// 0150) on mutations of the single directory programs (with `main`) of
// the repository that the Go checker accepts and that import only std:
// an operator swapped for another of its kind, an integer or Bool
// literal changed, a statement line dropped or doubled, or a `?`
// removed. Mutations either checker rejects are counted; the others are
// compared by the rules of TestSelfhostSSA. It makes 400 mutations (none
// with -short); XO_SELFHOST_SEED and XO_SELFHOST_SSA_FUZZ=N override. The Xo
// builder is built with the go backend, or the one XO_SELFHOST_BACKEND
// names.
func TestSelfhostSSAMutations(t *testing.T) {
	if testing.Short() {
		t.Skip("checks every repository program to find the sources (about 6 s)")
	}
	n := 400
	if v := os.Getenv("XO_SELFHOST_SSA_FUZZ"); v != "" {
		fmt.Sscan(v, &n)
	}
	seed := int64(20261010)
	if v := os.Getenv("XO_SELFHOST_SEED"); v != "" {
		fmt.Sscan(v, &seed)
	}
	root := repoRoot(t)
	backend := "go"
	if v := os.Getenv("XO_SELFHOST_BACKEND"); v != "" {
		backend = v
	}
	bin, err := selfhostBinary("ssa", backend)
	if err != nil {
		t.Fatal(err)
	}
	var srcs []mutationSource
	for _, s := range mutationSources(t, root) {
		for _, f := range s.files {
			if bytes.Contains(f, []byte("fn main(")) {
				srcs = append(srcs, s)
				break
			}
		}
	}
	if len(srcs) < 30 {
		t.Fatalf("only %d programs", len(srcs))
	}
	tmp := t.TempDir()
	stdDir, err := filepath.Abs(filepath.Join(root, "std"))
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(seed))
	var dirs []string
	for i := range n {
		src := srcs[rng.Intn(len(srcs))]
		dir := fmt.Sprintf("s%04d", i)
		if err := os.MkdirAll(filepath.Join(tmp, dir), 0o755); err != nil {
			t.Fatal(err)
		}
		f := rng.Intn(len(src.files))
		for j, name := range src.names {
			data := src.files[j]
			if j == f {
				data = mutateCode(rng, data)
			}
			if err := os.WriteFile(filepath.Join(tmp, dir, name), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		dirs = append(dirs, dir)
	}
	t.Chdir(tmp)
	setModEnv(t, root)
	got := map[string]string{}
	const batch = 60
	texts := runXoBatches(t, bin, tmp, []string{"--each", "--std=" + stdDir, "--wd=" + workDir(t)}, dirs, batch)
	for k, text := range texts {
		i := k * batch
		chunk := dirs[i:min(i+batch, len(dirs))]
		parts := runHead.Split(text, -1)
		if len(parts) != len(chunk)+1 {
			t.Fatalf("Xo builder printed %d runs for %d directories", len(parts)-1, len(chunk))
		}
		for j, d := range chunk {
			got[d] = strings.TrimPrefix(parts[j+1], d+"\n")
		}
	}
	var st ssaStats
	var fails []string
	wd := workDir(t)
	wants := make([]goSSA, len(dirs))
	parallel(len(dirs), func(i int) { wants[i] = goSSABuild(wd, dirs[i]) })
	for i, d := range dirs {
		want := wants[i]
		for _, msg := range compareSSA(d, want, got[d], &st) {
			src, _ := os.ReadFile(filepath.Join(tmp, d, firstXo(t, filepath.Join(tmp, d))))
			fails = append(fails, msg+"\n"+firstLines(string(src), 0))
		}
	}
	t.Logf("%d mutations: %s", n, st)
	if len(fails) > 0 {
		if len(fails) > 10 {
			fails = append(fails[:10], fmt.Sprintf("... and %d more", len(fails)-10))
		}
		t.Fatalf("%d differences:\n%s", len(fails), strings.Join(fails, "\n"))
	}
}

// mutateCode applies one code edit to src: an operator swapped for
// another of its kind, an integer or Bool literal changed, a statement
// line dropped or doubled, or a `?` removed.
func mutateCode(rng *rand.Rand, src []byte) []byte {
	var ds diag.List
	toks := lexer.Lex("f.xo", src, &ds).Tokens
	swaps := map[token.Kind][]string{
		token.Plus: {"-", "*"}, token.Minus: {"+", "*"}, token.Star: {"+", "-"}, token.Slash: {"%", "*"}, token.Percent: {"/"},
		token.Lt: {"<=", ">", "=="}, token.LtEq: {"<", ">="}, token.Gt: {">=", "<"}, token.GtEq: {">", "<="},
		token.Eq: {"!="}, token.NotEq: {"=="}, token.AndAnd: {"||"}, token.OrOr: {"&&"},
	}
	var ops, ints, bools, quests []token.Token
	for i, tk := range toks {
		switch {
		case swaps[tk.Kind] != nil:
			ops = append(ops, tk)
		case tk.Kind == token.Int:
			ints = append(ints, tk)
		case tk.Kind == token.Ident && (tk.Text == "true" || tk.Text == "false"):
			bools = append(bools, tk)
		case tk.Kind == token.Question && i > 0 && toks[i-1].Span.End.Offset == tk.Span.Start.Offset:
			quests = append(quests, tk)
		}
	}
	replace := func(tk token.Token, with string) []byte {
		var b bytes.Buffer
		b.Write(src[:tk.Span.Start.Offset])
		b.WriteString(with)
		b.Write(src[tk.Span.End.Offset:])
		return b.Bytes()
	}
	lines := strings.Split(string(src), "\n")
	var stmts []int
	for i, l := range lines {
		s := strings.TrimSpace(l)
		// an indented line that is a whole statement on its own
		if strings.HasPrefix(l, "  ") && s != "" && !strings.HasSuffix(s, "{") && !strings.HasPrefix(s, "}") && !strings.HasPrefix(s, "//") && !strings.HasSuffix(s, ",") && !strings.HasSuffix(s, "(") {
			stmts = append(stmts, i)
		}
	}
	for range 6 {
		switch rng.Intn(6) {
		case 0, 1:
			if len(ops) > 0 {
				tk := ops[rng.Intn(len(ops))]
				alts := swaps[tk.Kind]
				return replace(tk, alts[rng.Intn(len(alts))])
			}
		case 2:
			if len(ints) > 0 {
				tk := ints[rng.Intn(len(ints))]
				v, err := strconv.ParseInt(strings.ReplaceAll(tk.Text, "_", ""), 0, 64)
				if err != nil {
					continue
				}
				return replace(tk, strconv.FormatInt([]int64{0, 1, v + 1, v * 3, 255}[rng.Intn(5)], 10))
			}
		case 3:
			if len(bools) > 0 {
				tk := bools[rng.Intn(len(bools))]
				if tk.Text == "true" {
					return replace(tk, "false")
				}
				return replace(tk, "true")
			}
		case 4:
			if len(stmts) > 0 {
				i := stmts[rng.Intn(len(stmts))]
				out := append([]string{}, lines[:i]...)
				if rng.Intn(2) == 0 {
					out = append(out, lines[i])
				}
				out = append(out, lines[i:]...)
				if len(out) > len(lines) {
					return []byte(strings.Join(out, "\n"))
				}
				return []byte(strings.Join(append(lines[:i:i], lines[i+1:]...), "\n"))
			}
		case 5:
			if len(quests) > 0 {
				return replace(quests[rng.Intn(len(quests))], "")
			}
		}
	}
	return src
}
