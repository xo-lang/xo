package selfhost

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/llvm"
)

// TestSelfhostLLVM runs the Xo LLVM emitter (selfhost/llvm, decision
// 0166), built with each backend, and internal/llvm's GenerateSSA on
// every directory of the repository that holds .xo files (the runs of
// TestSelfhostSSA), each checked as a native build checks it and lowered
// from `main`, and compares the IR:
//
//   - every function the Xo emitter lowers is identical, text for text,
//     to the Go emitter's definition of the same name (string literals
//     and fault locations compared by their contents, not their numbers);
//   - every helper both emit (`xo.dup.T`, `xo.eq.T`, environments) is
//     identical, and on a run with nothing skipped the helpers, named
//     types, and runtime declarations are the same sets;
//   - the whole module is compared byte for byte, and counted;
//   - a run the Go checker rejects is rejected by the Xo checker.
//
// Runs the Xo SSA builder does not build completely are counted, not
// compared (its comparison is TestSelfhostSSA); so are functions the
// emitter leaves to a later stage (`== skipped`). The test binaries of
// the runs with test blocks are compared the same way. The full tier
// takes every run; the merge tier every third run but the mergeHeavy
// ones (each takes about a minute), and -short every 25th.
// XO_SELFHOST_BACKEND names one backend,
// XO_SELFHOST_LLVM_STAGE=N lowers what the stages up to N cover,
// XO_SELFHOST_DIRS=<texts> takes the runs whose directory contains one
// of the comma separated texts instead.
func TestSelfhostLLVM(t *testing.T) {
	backends := tierBackends(t)
	if v := os.Getenv("XO_SELFHOST_BACKEND"); v != "" {
		backends = []string{v}
	}
	root := repoRoot(t)
	dirs := moduleDirs(t, root)
	switch selfhostTier(t) {
	case tierShort:
		var some []string
		for i, d := range dirs {
			if i%25 == 0 {
				some = append(some, d)
			}
		}
		dirs = some
	case tierMerge:
		// every third run, not the mergeHeavy ones
		var some []string
		for i, d := range dirs {
			if !mergeHeavy(d) && i%3 == 0 {
				some = append(some, d)
			}
		}
		dirs = some
	}
	if v := os.Getenv("XO_SELFHOST_DIRS"); v != "" {
		// the runs whose directory contains one of the comma separated
		// texts (a development loop, as TestSelfhostArm64)
		var some []string
		for _, d := range moduleDirs(t, root) {
			for _, w := range strings.Split(v, ",") {
				if strings.Contains(d, w) {
					some = append(some, d)
					break
				}
			}
		}
		dirs = some
	}
	bins := selfhostBinaries(t, "llvm", backends)
	t.Chdir(root)
	setModEnv(t, root)
	wants := make([]*goldPieces, len(dirs))
	parallel(len(dirs), func(i int) { wants[i] = goLLVM(root, dirs[i], false) })
	tdirs := testDirs(t, root, dirs)
	twants := make([]*goldPieces, len(tdirs))
	parallel(len(tdirs), func(i int) { twants[i] = goLLVM(root, tdirs[i], true) })
	flags := []string{"--std=std"}
	if v := os.Getenv("XO_SELFHOST_LLVM_STAGE"); v != "" {
		flags = append(flags, "--stage="+v)
	}
	for _, b := range backends {
		bin, ok := bins[b]
		if !ok {
			continue
		}
		t.Run(b, func(t *testing.T) {
			t.Parallel()
			got := runLLVM(t, bin, root, dirs, flags...)
			var st llvmStats
			var fails []string
			for i, d := range dirs {
				fails = append(fails, compareLLVM(d, wants[i], got[d], &st)...)
			}
			t.Logf("%s", st)
			tgot := runLLVM(t, bin, root, tdirs, append(flags, "--test")...)
			var tst llvmStats
			for i, d := range tdirs {
				fails = append(fails, compareLLVM("test "+d, twants[i], tgot[d], &tst)...)
			}
			t.Logf("test binaries: %s", tst)
			if keep := os.Getenv("XO_SELFHOST_KEEP"); keep != "" {
				os.MkdirAll(keep, 0o755)
				os.WriteFile(filepath.Join(keep, "llvm-"+b+".txt"), []byte(st.report()+"test binaries: "+tst.report()), 0o644)
			}
			reportFails(t, fails)
		})
	}
}

// TestSelfhostLLVMMutations compares the two emitters on mutations of
// the single directory programs with `main` (TestSelfhostSSAMutations'
// sources and edits) by the rules of TestSelfhostLLVM: 400 in the full
// tier, 30 in the merge tier, none with -short; XO_SELFHOST_SEED and
// XO_SELFHOST_LLVM_FUZZ=N override. The Xo emitter is built with the go
// backend, or the one XO_SELFHOST_BACKEND names.
func TestSelfhostLLVMMutations(t *testing.T) {
	if selfhostTier(t) == tierShort {
		t.Skip("checks every repository program to find the sources (about 6 s)")
	}
	n := byTier(t, 0, 30, 400)
	if v := os.Getenv("XO_SELFHOST_LLVM_FUZZ"); v != "" {
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
	bin, err := selfhostBinary("llvm", backend)
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
		dir := fmt.Sprintf("l%04d", i)
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
	wd := workDir(t)
	wants := make([]*goldPieces, len(dirs))
	parallel(len(dirs), func(i int) { wants[i] = goLLVM(wd, dirs[i], false) })
	skipRecording(t)
	got := runLLVM(t, bin, tmp, dirs, "--std="+stdDir)
	var st llvmStats
	var fails []string
	for i, d := range dirs {
		for _, msg := range compareLLVM(d, wants[i], got[d], &st) {
			src, _ := os.ReadFile(filepath.Join(tmp, d, firstXo(t, filepath.Join(tmp, d))))
			fails = append(fails, msg+"\n"+firstLines(string(src), 0))
		}
	}
	t.Logf("%d mutations: %s", n, st)
	reportFails(t, fails)
}

// goIR is the Go emitter's result for one run: the IR, or why there is
// none ("rejected", "holes", "no main", or "error <message>").
type goIR struct {
	ir   string
	none string
}

// goLLVM checks dir as a native build does and lowers it as `xo
// llvmdump` does from root (the test binary when tests is set), cut into
// the pieces compareLLVM compares: meta "none" (why there is no IR), or
// meta "split" (why the IR does not split), else "module" (the whole
// IR), "order" (the names of the definitions in order), "d:<name>" (each
// definition), "c:<name>" (a definition with its string literals and
// fault locations replaced by their contents, canonGlobals, when that
// changes it), and the sorted lists "types" and "rtdecls". Recorded as
// the llvmdump kind (gogoldens_test.go).
func goLLVM(root, dir string, tests bool) *goldPieces {
	wd := goldensWD()
	cx := newGoldenCtx(wd, root, dir)
	o := fmt.Sprintf("tests=%v lineroot=%s", tests, cx.norm(root))
	var key []string
	if goldensMode() != goldensLive {
		key = []string{o, cx.dirKey(wd, dir, true)}
	}
	flags := ""
	if tests {
		flags = "--test "
	}
	hint := goldHint("TestSelfhostLLVM(Mutations)?", "llvmdump "+flags+dir, "llvm", "--each --std=std --wd="+wd+" "+flags+dir, wd)
	return recordedPieces("llvmdump", dir+" "+o, hint, key, cx, func() *goldPieces { return llvmGoldPieces(goLLVMLive(root, dir, tests)) })
}

// llvmGoldPieces cuts the Go emitter's result into goLLVM's pieces.
func llvmGoldPieces(r goIR) *goldPieces {
	p := newPieces()
	if r.none != "" {
		p.setMeta("none", r.none)
		return p
	}
	g, err := splitIR(r.ir)
	if err != nil {
		p.setMeta("split", err.Error())
		return p
	}
	p.add("module", r.ir)
	p.add("order", strings.Join(g.Order, "\n"))
	gc := canonGlobals(g.Globals)
	p.addFamily("d")
	p.addFamily("c")
	for name, t := range g.Defs {
		p.addMember("d", name, t)
		if c := gc(t); c != t {
			p.addMember("c", name, c)
		}
	}
	p.addList("types", g.Types)
	p.addList("rtdecls", g.RTDecls)
	return p
}

// goLLVMLive is goLLVM with the Go emitter run.
func goLLVMLive(root, dir string, tests bool) goIR {
	res, ok, err := build.CheckNative([]string{dir}, false)
	if err != nil {
		return goIR{none: "error " + err.Error()}
	}
	if !ok {
		holesOnly := true
		for _, d := range res.Diagnostics {
			if d.Severity == diag.Error && d.Code != "XO0900" {
				holesOnly = false
			}
		}
		if holesOnly {
			return goIR{none: "holes"}
		}
		return goIR{none: "rejected"}
	}
	prog, err := llvm.Generate(res, llvm.Options{LineRoot: root, Tests: tests})
	if err != nil {
		if strings.Contains(err.Error(), "no `main`") {
			return goIR{none: "no main"}
		}
		return goIR{none: "error " + err.Error()}
	}
	return goIR{ir: string(prog.IR)}
}

var llvmHead = regexp.MustCompile(`(?m)^=== llvm `)

// runLLVM runs the Xo emitter on every dir (--each) from root and splits
// its output by run.
func runLLVM(t *testing.T, bin, root string, dirs []string, flags ...string) map[string]string {
	out := map[string]string{}
	const batch = 40
	texts := runXoBatches(t, bin, root, append([]string{"--each", "--wd=" + workDir(t)}, flags...), dirs, batch)
	for k, text := range texts {
		i := k * batch
		chunk := dirs[i:min(i+batch, len(dirs))]
		parts := llvmHead.Split(text, -1)
		if len(parts) != len(chunk)+1 {
			t.Fatalf("Xo emitter printed %d runs for %d directories", len(parts)-1, len(chunk))
		}
		for j, d := range chunk {
			head := d + "\n"
			if !strings.HasPrefix(parts[j+1], head) {
				t.Fatalf("Xo emitter output for %s starts with %q", d, firstLine(parts[j+1]))
			}
			out[d] = strings.TrimPrefix(parts[j+1], head)
		}
	}
	return out
}

// llvmStats counts what compareLLVM compared.
type llvmStats struct {
	runs, compared, rejected, noMain, unsupported, ssaIncomplete, goErrors int
	// the Go emitter's functions (`x.*` and xo_entry) in the runs
	// compared: identical, skipped by the Xo emitter, differing
	funcs, same, skipped, differ int
	// helpers identical; modules byte identical; modules whose
	// definitions come in the same order
	helpers, modules, ordered int
	skips                     map[string]int
	why                       map[string]int
	// the runs compared whose modules are not byte identical
	moduleDiffs []string
}

func (s llvmStats) String() string {
	return fmt.Sprintf("%d runs: %d lowered by both and compared, %d rejected by both, %d without main, %d unsupported (use go, use c, holes), "+
		"%d the Xo SSA builder does not build completely, %d the Go emitter reports an error for; "+
		"the Go emitter's %d functions in the runs compared: %d identical, %d skipped by the Xo emitter, %d differing; "+
		"%d helpers identical; %d modules byte identical, %d with the definitions in the same order",
		s.runs, s.compared, s.rejected, s.noMain, s.unsupported, s.ssaIncomplete, s.goErrors,
		s.funcs, s.same, s.skipped, s.differ, s.helpers, s.modules, s.ordered)
}

// report is the counts, then why functions were skipped and runs not
// compared, most first.
func (s llvmStats) report() string {
	var b strings.Builder
	b.WriteString(s.String() + "\n")
	for _, m := range []map[string]int{s.why, s.skips} {
		var keys []string
		for k := range m {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return m[keys[i]] > m[keys[j]] || m[keys[i]] == m[keys[j]] && keys[i] < keys[j] })
		for _, k := range keys {
			fmt.Fprintf(&b, "%6d %s\n", m[k], k)
		}
	}
	for _, d := range s.moduleDiffs {
		b.WriteString("module not byte identical: " + d + "\n")
	}
	return b.String()
}

// isFunc reports a definition of a function of the module (not a
// helper): `x.<name>` or the entry.
func isFunc(name string) bool { return strings.HasPrefix(name, "x.") || name == "xo_entry" }

var globalRef = regexp.MustCompile(`@\.(rs|loc)\.\d+\b`)

// canonGlobals replaces the numbered names of string literals and fault
// locations in text by their definitions (globals: the lines that
// define them), so texts compare by contents.
func canonGlobals(globals []string) func(string) string {
	def := map[string]string{}
	for _, l := range globals {
		name, rest, ok := strings.Cut(strings.TrimSuffix(l, "\n"), " = ")
		if ok {
			def[name] = "@<" + rest + ">"
		}
	}
	return func(text string) string {
		return globalRef.ReplaceAllStringFunc(text, func(n string) string {
			if d, ok := def[n]; ok {
				return d
			}
			return n
		})
	}
}

// compareLLVM compares one run and returns its differences.
func compareLLVM(dir string, want *goldPieces, got string, st *llvmStats) []string {
	st.runs++
	none := want.m("none")
	if want.missing {
		none = want.missingText()
	}
	if st.skips == nil {
		st.skips, st.why = map[string]int{}, map[string]int{}
	}
	head := firstLine(got)
	switch {
	case none == "holes" || strings.HasPrefix(head, "== unsupported use go") || strings.HasPrefix(head, "== unsupported use c"):
		st.unsupported++
		st.why["run: "+strings.TrimPrefix(head, "== ")]++
		return nil
	case none == "rejected":
		if head != "== rejected" && !strings.HasPrefix(head, "== unsupported ") {
			return []string{dir + ": the Go checker rejects it, the Xo checker does not: " + head}
		}
		st.rejected++
		return nil
	case head == "== rejected":
		return []string{dir + ": the Xo checker rejects it, the Go checker does not"}
	case none == "no main":
		if !strings.Contains(head, "no `main`") {
			return []string{dir + ": no main for the Go builder, the Xo emitter prints " + head}
		}
		st.noMain++
		return nil
	case strings.HasPrefix(head, "== unsupported "):
		// the Xo SSA builder skipped functions: TestSelfhostSSA compares
		// them; the module is not complete, so not lowered
		st.ssaIncomplete++
		w := strings.TrimPrefix(head, "== unsupported ")
		if i := strings.Index(w, " ("); i >= 0 {
			w = w[:i]
		}
		st.why["run: "+w]++
		return nil
	case strings.HasPrefix(none, "error "):
		if strings.HasPrefix(head, "== error ") {
			st.goErrors++
			return nil
		}
		return []string{dir + ": the Go emitter reports " + firstLine(none) + "; the Xo emitter prints " + head}
	case none != "":
		return []string{dir + ": the Go emitter: " + none}
	case head != "== ir":
		return []string{dir + ": the Xo emitter prints " + head}
	}
	st.compared++
	body, skipped, _ := strings.Cut(strings.TrimPrefix(got, "== ir\n"), "== skipped\n")
	skip := map[string]string{}
	for _, l := range nonEmptyLines(skipped) {
		// the last ": ": a name may hold one (`xo.frameargs(req: T)`),
		// the reasons do not
		i := strings.LastIndex(l, ": ")
		if i < 0 {
			return []string{dir + ": a skipped line without a reason: " + l}
		}
		skip[l[:i]] = l[i+2:]
	}
	if e := want.m("split"); e != "" {
		return []string{dir + ": the Go emitter's IR: " + e}
	}
	x, err := splitIR(body)
	if err != nil {
		return []string{dir + ": the Xo emitter's IR: " + err.Error()}
	}
	xc := canonGlobals(x.Globals)
	xcanon := map[string]string{}
	for name, t := range x.Defs {
		if c := xc(t); c != t {
			xcanon[name] = c
		}
	}
	var xskipped []string
	for fn := range skip {
		xskipped = append(xskipped, "x."+fn)
	}
	sort.Strings(xskipped)
	// the Go side's splitIR (llvmGoldPieces)
	g := want.resolved("d", x.Defs, xskipped).resolved("c", xcanon, nil)
	if g.eq("module", body) {
		st.modules++
	} else if t, ok := g.text("module"); ok {
		st.moduleDiffs = append(st.moduleDiffs, dir+": "+firstDiff(body, t))
	} else {
		st.moduleDiffs = append(st.moduleDiffs, dir+": the hash differs from the recorded Go module's")
	}
	if g.eq("order", strings.Join(x.Order, "\n")) {
		st.ordered++
	}
	// canon is the piece of a definition compared by contents
	canon := func(name string) string {
		if g.has("c:" + name) {
			return "c:" + name
		}
		return "d:" + name
	}
	same := func(name, xt string) bool {
		return g.eq("d:"+name, xt) || g.eq(canon(name), xc(xt))
	}
	var fails []string
	for _, name := range g.names("d:") {
		xt, ok := x.Defs[name]
		if !isFunc(name) {
			switch {
			case ok && same(name, xt):
				st.helpers++
			case ok:
				fails = append(fails, fmt.Sprintf("%s: helper %s differs: %s", dir, name, g.diff(canon(name), xc(xt))))
			case len(skip) == 0:
				fails = append(fails, fmt.Sprintf("%s: the Xo emitter has no helper %s", dir, name))
			}
			continue
		}
		st.funcs++
		fn := strings.TrimPrefix(name, "x.")
		switch {
		case ok && same(name, xt):
			st.same++
		case ok:
			st.differ++
			fails = append(fails, fmt.Sprintf("%s: %s differs: %s", dir, name, g.diff(canon(name), xc(xt))))
		case skip[fn] != "":
			st.skipped++
			st.skips[skip[fn]]++
		default:
			st.differ++
			fails = append(fails, fmt.Sprintf("%s: the Xo emitter neither lowers nor skips %s", dir, name))
		}
	}
	for _, name := range x.Order {
		if !g.has("d:" + name) {
			fails = append(fails, fmt.Sprintf("%s: the Xo emitter defines %s, the Go emitter does not", dir, name))
		}
	}
	if len(skip) == 0 {
		for _, part := range []struct {
			what, name string
			x          []string
		}{{"named types", "types", x.Types}, {"runtime declarations", "rtdecls", x.RTDecls}} {
			if !g.eqList(part.name, part.x) {
				fails = append(fails, fmt.Sprintf("%s: the %s differ: %s", dir, part.what, g.diffList(part.name, part.x)))
			}
		}
	}
	sort.Strings(fails)
	return fails
}
