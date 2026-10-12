package selfhost

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/gen"
)

// genFlags are the flags both generators run with: the default lowering
// (monomorphization, the range analysis, arenas), and one run in genAlt of
// the others with monomorphization and the range analysis off and a
// release build without error arguments (every run in the full tier).
var genFlags = []string{}

var genAlt = []string{"--no-mono", "--no-ranges", "--release", "--no-error-args"}

// genHeavy are the runs whose generation takes the longest: the merge
// tier leaves them to the full tier.
var genHeavy = map[string]bool{
	"selfhost/check":  true,
	"selfhost/ssa":    true,
	"selfhost/parser": true,
	"selfhost/arm64":  true,
	"selfhost/gen":    true,
}

// TestSelfhostGen runs the Xo Go code generator (selfhost/gen, decision
// 0167), built with each backend, and internal/gen (build.GoGenDump, the
// text of `xo gendump`) on every directory of the repository holding .xo
// files, as programs and, for those with test blocks, as test binaries,
// and compares the generated Go: every file after internal/gen's last
// step (the Xo generator's own with --prep, blank uses and repeated
// //line directives dropped, then gofmt here), cut into top level
// declarations (genDecls) with the temporaries renumbered; every
// declaration the Xo generator writes must be identical to the Go
// generator's of the same name, and one it skips is counted, never
// compared. A run with no declaration skipped must be identical byte for
// byte. Every fourth run is also generated with genAlt (every run in the
// full tier). The merge tier takes every run but genHeavy; -short every 25th;
// the go backend only except in the full tier. XO_SELFHOST_BACKEND names
// one backend, XO_SELFHOST_KEEP=<dir> writes gen-<backend>.txt (the
// counts).
func TestSelfhostGen(t *testing.T) {
	backends := tierBackends(t)
	if v := os.Getenv("XO_SELFHOST_BACKEND"); v != "" {
		backends = []string{v}
	}
	root := repoRoot(t)
	all := moduleDirs(t, root)
	var dirs, tests []string
	step := byTier(t, 25, 1, 1)
	for i, d := range all {
		if i%step != 0 || (selfhostTier(t) != tierFull && genHeavy[d]) {
			continue
		}
		dirs = append(dirs, d)
		if hasTestBlock(filepath.Join(root, d)) {
			tests = append(tests, d)
		}
	}
	t.Chdir(root)
	setModEnv(t, root)
	altStep := byTier(t, 1, 4, 1)
	var alts []string
	for i, d := range dirs {
		if i%altStep == 0 {
			alts = append(alts, d)
		}
	}
	wants := make([]string, len(dirs))
	twants := make([]string, len(tests))
	awants := make([]string, len(alts))
	// the Go generator's results, while the subtests run the Xo one
	wait := background(t, func() {
		all := append(append(append([]string(nil), dirs...), tests...), alts...)
		parallel(t, "Go generator", all, func(i int) {
			switch {
			case i < len(dirs):
				wants[i] = goGen(dirs[i], genFlags, false)
			case i < len(dirs)+len(tests):
				i -= len(dirs)
				twants[i] = goGen(tests[i], genFlags, true)
			default:
				i -= len(dirs) + len(tests)
				awants[i] = goGen(alts[i], genAlt, false)
			}
		})
	})
	bins := selfhostBinaries(t, "gen", backends)
	for _, b := range backends {
		bin, ok := bins[b]
		if !ok {
			continue
		}
		t.Run(b, func(t *testing.T) {
			t.Parallel()
			got := runGen(t, bin, root, dirs, genFlags)
			tgot := runGen(t, bin, root, tests, append([]string{"--test"}, genFlags...))
			agot := runGen(t, bin, root, alts, genAlt)
			wait()
			var st genStats
			var fails []string
			for i, d := range dirs {
				fails = append(fails, compareGen(d, wants[i], got[d], &st)...)
			}
			for i, d := range tests {
				fails = append(fails, compareGen(d+" (test)", twants[i], tgot[d], &st)...)
			}
			for i, d := range alts {
				fails = append(fails, compareGen(d+" (no-mono, no-ranges, release)", awants[i], agot[d], &st)...)
			}
			t.Logf("%s", st)
			if keep := os.Getenv("XO_SELFHOST_KEEP"); keep != "" {
				os.MkdirAll(keep, 0o755)
				os.WriteFile(filepath.Join(keep, "gen-"+b+".txt"), []byte(st.report()), 0o644)
			}
			reportFails(t, fails)
		})
	}
}

// TestSelfhostGenMutations compares the two generators on mutations of
// the single directory programs with `main` (mutateCode, as
// TestSelfhostSSAMutations): 300 in the full tier, 60 in the merge tier,
// none with -short; XO_SELFHOST_GEN_FUZZ=N and XO_SELFHOST_SEED override.
func TestSelfhostGenMutations(t *testing.T) {
	if selfhostTier(t) == tierShort {
		t.Skip("checks every repository program to find the sources")
	}
	n := byTier(t, 0, 60, 300)
	if v := os.Getenv("XO_SELFHOST_GEN_FUZZ"); v != "" {
		fmt.Sscan(v, &n)
	}
	seed := int64(20261011)
	if v := os.Getenv("XO_SELFHOST_SEED"); v != "" {
		fmt.Sscan(v, &seed)
	}
	root := repoRoot(t)
	backend := "go"
	if v := os.Getenv("XO_SELFHOST_BACKEND"); v != "" {
		backend = v
	}
	bin, err := selfhostBinary("gen", backend)
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
	tmp := t.TempDir()
	stdDir, err := filepath.Abs(filepath.Join(root, "std"))
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(seed))
	var dirs []string
	for i := range n {
		src := srcs[rng.Intn(len(srcs))]
		dir := fmt.Sprintf("g%04d", i)
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
	if goldensRecording() {
		for _, d := range dirs {
			goGen(d, genFlags, false)
		}
		skipRecording(t)
	}
	wants := make([]string, len(dirs))
	wait := background(t, func() {
		parallel(t, "Go generator", dirs, func(i int) { wants[i] = goGen(dirs[i], genFlags, false) })
	})
	got := runGen(t, bin, tmp, dirs, genFlags, "--std="+stdDir)
	wait()
	var st genStats
	var fails []string
	for i, d := range dirs {
		fails = append(fails, compareGen(d, wants[i], got[d], &st)...)
	}
	t.Logf("%d mutations: %s", n, st)
	reportFails(t, fails)
}

// hasTestBlock reports whether a .xo file of dir declares a test block.
func hasTestBlock(dir string) bool {
	files, _ := filepath.Glob(filepath.Join(dir, "*.xo"))
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err == nil && regexp.MustCompile(`(?m)^test "`).Match(data) {
			return true
		}
	}
	return false
}

// hasHole reports whether a .xo file of dir (a repository run, or a
// mutation relative to the working directory) holds a hole `???`.
func hasHole(dir string) bool {
	dir, _, _ = strings.Cut(dir, " (")
	files, _ := filepath.Glob(filepath.Join(dir, "*.xo"))
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err == nil && bytes.Contains(data, []byte("???")) {
			return true
		}
	}
	return false
}

// goGen is what `xo gendump` prints for dir with flags. Recorded as the
// gendump kind (gogoldens_test.go).
func goGen(dir string, flags []string, test bool) string {
	if goldensMode() == goldensLive {
		return goGenLive(dir, flags, test)
	}
	wd := goldensWD()
	cx := newGoldenCtx(wd, dir)
	o := fmt.Sprintf("flags=%s test=%v", strings.Join(flags, ","), test)
	return recorded("gendump", dir+" "+o, []string{o, cx.dirKey(wd, dir, false)}, cx, func() string { return goGenLive(dir, flags, test) })
}

// goGenLive is goGen with internal/gen run.
func goGenLive(dir string, flags []string, test bool) string {
	opts := gen.Options{Mode: gen.Build, Arenas: true}
	if test {
		opts.Mode = gen.Test
	}
	for _, f := range flags {
		switch f {
		case "--no-mono":
			opts.NoMono = true
		case "--no-ranges":
			opts.NoRanges = true
		case "--no-arenas":
			opts.Arenas = false
		case "--release":
			opts.Release = true
		case "--no-error-args":
			opts.NoErrorArgs = true
		}
	}
	return build.GoGenDump([]string{dir}, opts)
}

var genHead = regexp.MustCompile(`(?m)^=== gen `)

// runGen runs the Xo generator on every dir (--each) from root and splits
// its output by run.
func runGen(t *testing.T, bin, root string, dirs []string, flags []string, more ...string) map[string]string {
	out := map[string]string{}
	if len(dirs) == 0 {
		return out
	}
	const batch = 20
	args := append(append([]string{"--each", "--prep", "--wd=" + workDir(t)}, flags...), more...)
	if len(more) == 0 {
		args = append(args, "--std=std")
	}
	for _, xb := range runXoBatches(t, bin, root, args, dirs, batch) {
		chunk := xb.paths
		parts := genHead.Split(xb.out, -1)
		if len(parts) != len(chunk)+1 {
			t.Fatalf("Xo generator printed %d runs for %d directories", len(parts)-1, len(chunk))
		}
		for j, d := range chunk {
			head := d + "\n"
			if !strings.HasPrefix(parts[j+1], head) {
				t.Fatalf("run %d is not %s", j, d)
			}
			out[d] = strings.TrimPrefix(parts[j+1], head)
		}
	}
	return out
}

type genStats struct {
	runs, compared, rejected, unsupported, errors, byteSame, holes int
	decls, same, skipped, afterSkip                                int
	skips                                                          map[string]int
}

func (s genStats) String() string {
	return fmt.Sprintf("%d runs: %d compared, %d rejected by both, %d with holes, %d not checked by the Xo checker, %d failed in both generators; "+
		"%d runs byte identical; the Go generator's %d declarations in the runs compared: %d identical, %d skipped by the Xo generator, %d not written after a skip",
		s.runs, s.compared, s.rejected, s.holes, s.unsupported, s.errors, s.byteSame, s.decls, s.same, s.skipped, s.afterSkip)
}

func (s genStats) report() string {
	var b strings.Builder
	b.WriteString(s.String() + "\n")
	var keys []string
	for k := range s.skips {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return s.skips[keys[i]] > s.skips[keys[j]] || s.skips[keys[i]] == s.skips[keys[j]] && keys[i] < keys[j]
	})
	for _, k := range keys {
		fmt.Fprintf(&b, "%6d %s\n", s.skips[k], k)
	}
	return b.String()
}

// genFiles cuts a dump into its files (path to text) and the skipped
// lines that follow `== skipped`.
func genFiles(text string) (map[string]string, []string) {
	// the markers start lines; generated Go may hold their text elsewhere
	// (selfhost/gen's own dump code)
	files := map[string]string{}
	var skipped []string
	path, inSkipped := "", false
	var cur strings.Builder
	flush := func() {
		if path != "" {
			files[path] = cur.String()
		}
		cur.Reset()
	}
	for _, l := range strings.SplitAfter(text, "\n") {
		switch {
		case strings.HasPrefix(l, "== file "):
			flush()
			path = strings.TrimSuffix(strings.TrimPrefix(l, "== file "), "\n")
		case l == "== skipped\n":
			flush()
			path, inSkipped = "", true
		case inSkipped:
			if s := strings.TrimSuffix(l, "\n"); s != "" {
				skipped = append(skipped, s)
			}
		default:
			cur.WriteString(l)
		}
	}
	flush()
	return files, skipped
}

// compareGen compares one run and returns its differences.
func compareGen(dir, want, got string, st *genStats) []string {
	st.runs++
	if st.skips == nil {
		st.skips = map[string]int{}
	}
	// --prep: the Xo generator applied internal/gen's last step but gofmt
	got, prepped := strings.CutPrefix(got, "== prepped\n")
	gh, xh := firstLine(want), firstLine(got)
	switch {
	case strings.HasPrefix(xh, "== unsupported"):
		st.unsupported++
		return nil
	case gh == "== rejected":
		if xh != "== rejected" && hasHole(dir) {
			// holes (XO0900) the Xo checker does not report (decision
			// 0141)
			st.holes++
			return nil
		}
		if xh != "== rejected" {
			return []string{dir + ": the Go checker rejects it, the Xo checker does not: " + xh}
		}
		st.rejected++
		return nil
	case xh == "== rejected":
		return []string{dir + ": the Xo checker rejects it, the Go checker does not"}
	case strings.HasPrefix(gh, "== error"):
		if gh != xh {
			return []string{fmt.Sprintf("%s: the Go generator fails (%s), the Xo generator: %s", dir, gh, xh)}
		}
		st.errors++
		return nil
	case strings.HasPrefix(xh, "== error"):
		return []string{dir + ": the Xo generator: " + xh}
	}
	st.compared++
	gfiles, _ := genFiles(want)
	xraw, skipped := genFiles(got)
	skip := map[string]string{}
	skipFiles := map[string]bool{}
	for _, l := range skipped {
		// `<file> <declaration>: <why>`
		file, rest, _ := strings.Cut(l, " ")
		key, why, _ := strings.Cut(rest, ": ")
		skip[file+" "+key] = why
		skipFiles[file] = true
	}
	var fails []string
	xfiles := map[string]string{}
	for p, raw := range xraw {
		if !strings.HasSuffix(p, ".go") {
			xfiles[p] = raw
			continue
		}
		var src []byte
		var err error
		if prepped {
			src, err = format.Source([]byte(raw))
		} else {
			src, err = gen.FormatFile([]byte(raw))
		}
		if err != nil {
			fails = append(fails, fmt.Sprintf("%s: %s from the Xo generator does not format: %v", dir, p, err))
			continue
		}
		xfiles[p] = string(src)
	}
	same := len(fails) == 0 && len(skipped) == 0 && len(gfiles) == len(xfiles)
	for p, g := range gfiles {
		if xfiles[p] != g {
			same = false
		}
	}
	if same {
		st.byteSame++
	} else if len(skipped) == 0 && len(fails) == 0 {
		// with no declaration skipped every file must be the Go
		// generator's byte for byte (decision 0167)
		for _, p := range sortedKeys(gfiles) {
			x, ok := xfiles[p]
			if !ok {
				fails = append(fails, fmt.Sprintf("%s: the Xo generator writes no %s", dir, p))
				continue
			}
			if x != gfiles[p] {
				fails = append(fails, fmt.Sprintf("%s: %s differs: %s", dir, p, firstDiff(x, gfiles[p])))
			}
		}
	}
	for _, p := range sortedKeys(gfiles) {
		gd := genDecls(gfiles[p])
		x, ok := xfiles[p]
		if !ok {
			if !strings.HasSuffix(p, ".go") || len(fails) > 0 {
				continue
			}
			fails = append(fails, fmt.Sprintf("%s: the Xo generator writes no %s", dir, p))
			continue
		}
		xd := genDecls(x)
		for _, k := range sortedKeys(gd) {
			st.decls++
			xt, ok := xd[k]
			switch {
			case ok && xt == gd[k]:
				st.same++
			case ok:
				fails = append(fails, fmt.Sprintf("%s: %s %s differs: %s", dir, p, k, firstDiff(xt, gd[k])))
			case skip[p+" "+k] != "":
				st.skipped++
				st.skips[skip[p+" "+k]]++
			case skipFiles[p]:
				st.afterSkip++
			default:
				fails = append(fails, fmt.Sprintf("%s: %s: the Xo generator has no %s", dir, p, k))
			}
		}
		for _, k := range sortedKeys(xd) {
			if _, ok := gd[k]; !ok {
				fails = append(fails, fmt.Sprintf("%s: %s: the Xo generator writes %s, the Go generator does not", dir, p, k))
			}
		}
	}
	for p := range xfiles {
		if _, ok := gfiles[p]; !ok {
			fails = append(fails, fmt.Sprintf("%s: the Xo generator writes %s, the Go generator does not", dir, p))
		}
	}
	sort.Strings(fails)
	return fails
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// genTemp matches the temporaries the generators number per package.
var genTemp = regexp.MustCompile(`\bx_[A-Za-z]+[0-9]+\b`)

// genDecls cuts a Go file into its top level declarations, each with the
// //line directive and comments before it, keyed by name (`Recv.Name` for
// a method, the receiver type without type arguments); temporaries are
// renumbered in order of appearance within each declaration, so a
// declaration skipped earlier in the package shifts no other. The text
// before the first declaration (package clause, imports) is "header", a
// file that is not Go is "file".
func genDecls(src string) map[string]string {
	out := map[string]string{}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		out["file"] = src
		return out
	}
	tf := fset.File(f.Pos())
	prev := 0
	for i, d := range f.Decls {
		start := tf.Offset(d.Pos())
		// the comments and directives right before the declaration
		ls := strings.LastIndex(src[:start], "\n\n") + 2
		if ls < prev {
			ls = prev
		}
		if i == 0 {
			out["header"] = src[:ls]
		}
		end := tf.Offset(d.End())
		key := declKey(d)
		if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.IMPORT {
			continue
		}
		text := src[ls:end]
		seen := map[string]string{}
		text = genTemp.ReplaceAllStringFunc(text, func(s string) string {
			base := strings.TrimRight(s, "0123456789")
			if _, ok := seen[s]; !ok {
				seen[s] = fmt.Sprintf("%s#%d", base, len(seen))
			}
			return seen[s]
		})
		for _, ok := out[key]; ok; _, ok = out[key] {
			key += "'"
		}
		out[key] = text
		prev = end
	}
	return out
}

func declKey(d ast.Decl) string {
	switch x := d.(type) {
	case *ast.FuncDecl:
		if x.Recv != nil && len(x.Recv.List) > 0 {
			t := x.Recv.List[0].Type
			if s, ok := t.(*ast.StarExpr); ok {
				t = s.X
			}
			switch r := t.(type) {
			case *ast.IndexExpr:
				t = r.X
			case *ast.IndexListExpr:
				t = r.X
			}
			if id, ok := t.(*ast.Ident); ok {
				return id.Name + "." + x.Name.Name
			}
		}
		return x.Name.Name
	case *ast.GenDecl:
		if len(x.Specs) > 0 {
			switch s := x.Specs[0].(type) {
			case *ast.TypeSpec:
				return s.Name.Name
			case *ast.ValueSpec:
				if len(s.Names) > 0 {
					return s.Names[0].Name
				}
			}
		}
	}
	return "?"
}
