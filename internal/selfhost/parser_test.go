package selfhost

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/gen"
	"github.com/xo-lang/xo/internal/lexer"
	"github.com/xo-lang/xo/internal/parser"
	"github.com/xo-lang/xo/internal/progen"
)

// TestSelfhostParserDifferential runs the Xo parser (selfhost/parser),
// built with each backend, and the Go parser (internal/parser) on every
// .xo file of the repository (std declaration files in signature mode),
// the spec's code blocks, generated programs (internal/progen, decision
// 0110), and token level mutations of repository files, and requires
// (decision 0127):
//
//   - every file, accepted or rejected, has a byte identical dump (`xo
//     astdump`, parser.Dump): the tree, recovered after errors, and the
//     diagnostics; after a hard error both parsers resynchronize at the
//     end of the statement, member, arm, or declaration.
//
// -short uses fewer generated and mutated inputs, and so do the arm64
// and LLVM parsers in the merge tier (tier_test.go). XO_SELFHOST_SEED,
// XO_SELFHOST_PARSE_FUZZ=N, XO_SELFHOST_PROGS=N, and XO_SELFHOST_KEEP=dir
// as for the lexer.
func TestSelfhostParserDifferential(t *testing.T) {
	for _, backend := range []string{"go", "arm64", "llvm"} {
		t.Run(backend, func(t *testing.T) {
			t.Parallel()
			if backend != "go" {
				if goldensRecording() {
					t.Skip("XO_SELFHOST_GOLDENS=record: the go backend's run records the Go side")
				}
				if why := nativeSkip(backend); why != "" {
					t.Skip(why)
				}
			}
			bin, err := selfhostBinary("parser", backend)
			if err != nil {
				t.Fatal(err)
			}
			parserDifferential(t, bin, backend)
		})
	}
}

func parserDifferential(t *testing.T, bin, backend string) {
	root := repoRoot(t)
	var files, std []string
	for _, f := range repoXoFiles(t, root) {
		if strings.HasPrefix(f, "std/") {
			std = append(std, f)
		} else {
			files = append(files, f)
		}
	}
	tmp := t.TempDir()
	if keep := os.Getenv("XO_SELFHOST_KEEP"); keep != "" {
		tmp = filepath.Join(keep, "parser-"+filepath.Base(t.Name()))
		if err := os.MkdirAll(tmp, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("repo", func(t *testing.T) {
		acc, rej, rec := compareParsers(t, bin, root, files, false)
		t.Logf("%d repository files: %d accepted, %d rejected by both parsers (%d with hard errors), all identical", len(files), acc, rej, rec)
		// Every program of the repository parses except the habit and
		// error fixtures under testdata.
		if acc < 600 {
			t.Fatalf("only %d of %d repository files accepted", acc, len(files))
		}
	})
	t.Run("std", func(t *testing.T) {
		acc, _, _ := compareParsers(t, bin, root, std, true)
		if acc != len(std) {
			t.Fatalf("%d of %d std files accepted", acc, len(std))
		}
	})
	t.Run("spec", func(t *testing.T) {
		spec := specExamples(t, root, tmp)
		acc, rej, rec := compareParsers(t, bin, root, spec, false)
		t.Logf("%d spec blocks: %d accepted, %d rejected by both parsers (%d with hard errors)", len(spec), acc, rej, rec)
	})
	progs := byTier(t, 24, 150, 150)
	fuzz := byTier(t, 300, 2000, 2000)
	if backend != "go" && selfhostTier(t) == tierMerge {
		// the native smoke test of the merge tier: -short's sizes
		progs, fuzz = 24, 300
	}
	if v := os.Getenv("XO_SELFHOST_PROGS"); v != "" {
		fmt.Sscan(v, &progs)
	}
	if v := os.Getenv("XO_SELFHOST_PARSE_FUZZ"); v != "" {
		fmt.Sscan(v, &fuzz)
	}
	t.Run("progen", func(t *testing.T) {
		var paths []string
		for seed := 1; seed <= progs; seed++ {
			p := filepath.Join(tmp, fmt.Sprintf("prog-%04d.xo", seed))
			if err := os.WriteFile(p, []byte(progen.Program(int64(seed))), 0o644); err != nil {
				t.Fatal(err)
			}
			paths = append(paths, p)
		}
		acc, _, _ := compareParsers(t, bin, root, paths, false)
		if acc != len(paths) {
			t.Fatalf("%d of %d generated programs accepted", acc, len(paths))
		}
	})
	t.Run("fuzz", func(t *testing.T) {
		acc, rej, rec := compareParsers(t, bin, root, tokenFuzzCorpus(t, root, tmp, files, fuzz), false)
		t.Logf("%d mutated inputs: %d accepted, %d rejected by both parsers (%d with hard errors)", fuzz, acc, rej, rec)
	})
	t.Run("habits", func(t *testing.T) {
		n := fuzz / 4
		acc, rej, rec := compareParsers(t, bin, root, habitCorpus(t, root, tmp, files, n), false)
		t.Logf("%d inputs with habits of other languages: %d accepted, %d rejected by both parsers (%d with hard errors)", n, acc, rej, rec)
		if rej-rec < n/4 {
			t.Fatalf("only %d of %d habit inputs have only habit diagnostics", rej-rec, n)
		}
	})
}

// habitCorpus writes n inputs under dir, each a repository file with one
// to three habit edits (habitEdits of the checker mutations: Go, Rust,
// Python, TypeScript, Kotlin, and Swift syntax the parser recovers from),
// seeded so failures reproduce.
func habitCorpus(t *testing.T, root, dir string, files []string, n int) []string {
	seed := int64(20261011)
	if v := os.Getenv("XO_SELFHOST_SEED"); v != "" {
		fmt.Sscan(v, &seed)
	}
	rng := rand.New(rand.NewSource(seed))
	var srcs [][]byte
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			t.Fatal(err)
		}
		if len(data) < 16<<10 {
			srcs = append(srcs, data)
		}
	}
	var out []string
	for i := range n {
		src := srcs[rng.Intn(len(srcs))]
		for range 1 + rng.Intn(3) {
			src = mutateHabits(rng, src)
		}
		p := filepath.Join(dir, fmt.Sprintf("phabit-%05d.xo", i))
		if err := os.WriteFile(p, src, 0o644); err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
	}
	return out
}

// compareParsers parses paths with both parsers in batches and fails on
// a disagreement: the Xo parser's dump must equal the Go parser's byte
// for byte, the tree recovered after errors and the diagnostics
// included. It returns how many files both accepted, both rejected, and,
// of those rejected, how many have a hard error (not only habits of
// other languages, decision 0087, and other soft errors).
func compareParsers(t *testing.T, bin, root string, paths []string, sig bool) (accepted, rejected, hard int) {
	t.Helper()
	mode := parser.Mode(0)
	if sig {
		mode = parser.SignatureMode
	}
	var fails []string
	// goParse is the Go parser's dump of p (recorded, gogoldens_test.go)
	goParse := func(p string) string {
		full := p
		if !filepath.IsAbs(p) {
			full = filepath.Join(root, p)
		}
		src, err := os.ReadFile(full)
		if err != nil {
			t.Fatal(err)
		}
		cx := newGoldenCtx(full)
		return recorded("astdump", p, []string{fmt.Sprint(sig), cx.fileKey(p, src)}, cx, func() string { return parser.Dump(p, src, mode) })
	}
	if goldensRecording() {
		for _, p := range paths {
			goParse(p)
		}
		skipRecording(t)
	}
	const batch = 100
	for i := 0; i < len(paths); i += batch {
		chunk := paths[i:min(i+batch, len(paths))]
		args := chunk
		if sig {
			args = append([]string{"--sig"}, chunk...)
		}
		got := splitDumps(t, runXo(t, bin, root, args), chunk)
		for j, p := range chunk {
			want := goParse(p)
			goRejects, xoRejects := strings.Contains(want, "\ndiag "), strings.Contains(got[j], "\ndiag ")
			switch {
			case goRejects && !xoRejects:
				fails = append(fails, fmt.Sprintf("%s: the Go parser rejects it, the Xo parser accepts it:\n%s", p, diagLines(want)))
			case xoRejects && !goRejects:
				fails = append(fails, fmt.Sprintf("%s: the Xo parser rejects it, the Go parser accepts it:\n%s", p, diagLines(got[j])))
			case got[j] != want:
				fails = append(fails, fmt.Sprintf("%s: Xo and Go parsers differ: %s", p, firstDiff(got[j], want)))
			case goRejects:
				rejected++
				if !habitOnly(want) {
					hard++
				}
			default:
				accepted++
			}
		}
	}
	if len(fails) > 0 {
		n, show := len(fails), 10
		if v := os.Getenv("XO_SELFHOST_FAILS"); v != "" {
			fmt.Sscan(v, &show)
		}
		if n > show {
			fails = append(fails[:show], fmt.Sprintf("... and %d more", n-show))
		}
		t.Fatalf("%d of %d files differ:\n%s", n, len(paths), strings.Join(fails, "\n"))
	}
	return accepted, rejected, hard
}

// habitCodeRE matches the diagnostics of the habits of other languages
// the Go parser recovers from (decision 0087): XO0177 (but `class`, a hard
// error that the Xo parser also resynchronizes from), XO0178 (a line that
// starts with a binary operator), XO0169 (`,` after an arm), XO0173 (`use`
// after a declaration), and the lexer errors a habit renames.
var habitCodeRE = regexp.MustCompile(`^diag (XO0177|XO0178|XO0169|XO0173) |^diag XO015[0-9] .*(is a Rust attribute|starts a comment in Python|annotation; Xo has none|is Swift string interpolation|holding text is a Python)`)

// habitOnly reports whether every diagnostic of a Go parser dump is a
// habit the parser recovers from.
func habitOnly(dump string) bool {
	n := 0
	for _, l := range strings.Split(dump, "\n") {
		if strings.HasPrefix(l, "diag ") {
			n++
			if !habitCodeRE.MatchString(l) {
				return false
			}
		}
	}
	return n > 0
}

// splitDumps cuts the output of the Xo parser into one dump per path.
func splitDumps(t *testing.T, out string, paths []string) []string {
	t.Helper()
	var res []string
	for i, p := range paths {
		head := "== " + p + "\n"
		if !strings.HasPrefix(out, head) {
			t.Fatalf("Xo parser output for %s does not start with %q", p, head)
		}
		end := len(out)
		if i+1 < len(paths) {
			next := strings.Index(out[len(head):], "\n== "+paths[i+1]+"\n")
			if next < 0 {
				t.Fatalf("Xo parser output has no dump for %s", paths[i+1])
			}
			end = len(head) + next + 1
		}
		res = append(res, out[:end])
		out = out[end:]
	}
	return res
}

func diagLines(dump string) string {
	var out []string
	for _, l := range strings.Split(dump, "\n") {
		if strings.HasPrefix(l, "diag ") {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// parserFragments are spliced between tokens of mutated inputs: brackets,
// separators, operators with their precedence edge cases, keywords, and
// habits of other languages the Go parser diagnoses.
var parserFragments = []string{
	"(", ")", "[", "]", "{", "}", "#{", ",", "\n", ";", ":", ".", "..", "..=", "?", "??", "!", "=", "=>", "->",
	"|", "||", "&&", "+", "-", "*", "/", "%", "<<", ">>", "&", "^", "~", "==", "!=", "<", ">", "<=", ">=",
	"+=", "-=", "&=", "is", "with", "as", "in", "fn", "if", "else", "match", "let", "var", "const", "for",
	"while", "return", "break", "continue", "defer", "scope", "select", "after", "uses", "requires",
	"ensures", "pub", "struct", "enum", "type", "test", "derive", "_", "x", "T", "Some", "None", "0", "1",
	`"s"`, `"${x}"`, "'a'", "1s", "1.5", `b"x"`, "???", "()", "(x)", "(x,)", "[]", "{}", "x: Int", "self",
	"++", "--", ":=", "::", "and", "or", "not", "throw", "try", "new", "lambda", "when", "switch", "await",
	"map[Int]Str", "List<Int>", "&x", "*p", "`", "x =>", "elif", "?:", "**", "===",
}

// tokenFuzzCorpus writes n inputs under dir, each a repository file with
// one token level edit (a third of them two or three) (delete, duplicate, or swap tokens, or
// insert a fragment), seeded so failures reproduce.
func tokenFuzzCorpus(t *testing.T, root, dir string, files []string, n int) []string {
	seed := int64(20261010)
	if v := os.Getenv("XO_SELFHOST_SEED"); v != "" {
		fmt.Sscan(v, &seed)
	}
	rng := rand.New(rand.NewSource(seed))
	var srcs [][]byte
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			t.Fatal(err)
		}
		if len(data) < 16<<10 {
			srcs = append(srcs, data)
		}
	}
	var out []string
	for i := range n {
		src := srcs[rng.Intn(len(srcs))]
		edits := 1
		if rng.Intn(3) == 0 {
			edits += 1 + rng.Intn(2)
		}
		for range edits {
			src = mutateTokens(rng, src)
		}
		p := filepath.Join(dir, fmt.Sprintf("pfuzz-%05d.xo", i))
		if err := os.WriteFile(p, src, 0o644); err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
	}
	return out
}

// mutateTokens applies one token level edit to src.
func mutateTokens(rng *rand.Rand, src []byte) []byte {
	var ds diag.List
	toks := lexer.Lex("f.xo", src, &ds).Tokens
	if len(toks) < 2 {
		return append(src, parserFragments[rng.Intn(len(parserFragments))]...)
	}
	i := rng.Intn(len(toks) - 1) // never the final EOF
	a, b := toks[i].Span.Start.Offset, toks[i].Span.End.Offset
	var buf bytes.Buffer
	switch rng.Intn(4) {
	case 0: // delete a token
		buf.Write(src[:a])
		buf.Write(src[b:])
	case 1: // duplicate a token
		buf.Write(src[:b])
		buf.WriteByte(' ')
		buf.Write(src[a:])
	case 2: // swap with the next token
		j := i + 1
		c, d := toks[j].Span.Start.Offset, toks[j].Span.End.Offset
		if j == len(toks)-1 || c < b {
			return src
		}
		buf.Write(src[:a])
		buf.Write(src[c:d])
		buf.Write(src[b:c])
		buf.Write(src[a:b])
		buf.Write(src[d:])
	default: // insert a fragment before the token
		buf.Write(src[:a])
		buf.WriteString(parserFragments[rng.Intn(len(parserFragments))])
		buf.WriteByte(' ')
		buf.Write(src[a:])
	}
	return buf.Bytes()
}

// TestSelfhostParserXoTests runs the Xo tests of selfhost/parser
// (`xo test`).
func TestSelfhostParserXoTests(t *testing.T) {
	if selfhostTier(t) == tierShort {
		t.Skip("builds and runs the Xo test binary")
	}
	dir := filepath.Join("..", "..", "selfhost", "parser")
	res, err := build.Build([]string{dir}, build.Options{Mode: gen.Test})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed {
		t.Fatalf("check failed: %+v", res.Diagnostics)
	}
	cmd := exec.Command(res.Binaries[build.Host()])
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("xo test selfhost/parser: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "0 failed") {
		t.Fatalf("unexpected test output:\n%s", out)
	}
}
