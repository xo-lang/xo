package selfhost

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestSelfhostSSA runs the Xo SSA builder (compiler/ssa), built with each
// backend, on every directory of the repository that holds .xo files,
// each checked as a native build checks it (std/native included) and
// built from `main`, and compares its build stage with the recording
// (the recording's `xo ssadump` until decision 0215, decision 0150 stage
// 1) function by function:
//
//   - every function the Xo builder completes is identical to the Go
//     builder's of the same name (the closures of a function, named
//     `<function>.fn<N>` by a counter of the whole build, are compared
//     under their rank among the closures of that function);
//   - every type line the Xo builder prints is one the recording prints;
//   - a run the recording rejects is rejected by the Xo checker, and one
//     without `main` is not built by either.
//
// Functions the Xo builder skips (a construct of a later stage) or never
// declares (the callees of skipped code) are counted, not compared. The
// coverage is logged. The go backend only, except in the full tier
// (tier_test.go); -short also takes every 25th run but the mergeHeavy ones. XO_SELFHOST_BACKEND
// names one backend.
func TestSelfhostSSA(t *testing.T) {
	skipDumpReplay(t)
	backends := tierBackends(t)
	if v := os.Getenv("XO_SELFHOST_BACKEND"); v != "" {
		// one backend (a development loop)
		backends = []string{v}
	}
	root := repoRoot(t)
	dirs := moduleDirs(t, root)
	if v := os.Getenv("XO_SELFHOST_DIRS"); v != "" {
		// the runs whose directory contains one of the comma separated
		// texts (a development loop)
		var some []string
		for _, d := range dirs {
			for _, w := range strings.Split(v, ",") {
				if strings.Contains(d, w) {
					some = append(some, d)
					break
				}
			}
		}
		dirs = some
	} else if selfhostTier(t) == tierShort {
		var some []string
		for i, d := range dirs {
			if i%25 == 0 && !mergeHeavy(d) {
				some = append(some, d)
			}
		}
		dirs = some
	}
	// the merge tier compares only the build stage of the runs whose
	// passes take minutes (mergeHeavy)
	buildOnly := map[string]bool{}
	if selfhostTier(t) == tierMerge {
		for _, d := range dirs {
			buildOnly[d] = mergeHeavy(d)
		}
	}
	t.Chdir(root)
	setModEnv(t, root)
	// the test binaries (stage 7) of the runs with test blocks
	tdirs := testDirs(t, root, dirs)
	bins := selfhostBinaries(t, "ssa", backends)
	for _, b := range backends {
		bin, ok := bins[b]
		if !ok {
			continue
		}
		t.Run(b, func(t *testing.T) {
			t.Parallel()
			got := runSSAStages(t, bin, root, dirs, buildOnly, "--std=std")
			want := map[string]goSSA{}
			for _, d := range dirs {
				want[d] = goSSABuildOpts(root, d, false, !buildOnly[d], got[d])
			}
			st, fails := compareSSAAll(t, "", dirs, want, got)
			t.Logf("%s", st)
			tgot := runSSAStages(t, bin, root, tdirs, buildOnly, "--std=std", "--test")
			twant := map[string]goSSA{}
			for _, d := range tdirs {
				twant[d] = goSSABuildOpts(root, d, true, !buildOnly[d], tgot[d])
			}
			tst, tfails := compareSSAAll(t, "test ", tdirs, twant, tgot)
			fails = append(fails, tfails...)
			t.Logf("test binaries: %s", tst)
			if keep := os.Getenv("XO_SELFHOST_KEEP"); keep != "" {
				os.MkdirAll(keep, 0o755)
				os.WriteFile(filepath.Join(keep, "ssa-"+b+".txt"), []byte(st.skipReport()), 0o644)
				os.WriteFile(filepath.Join(keep, "ssa-test-"+b+".txt"), []byte(tst.skipReport()), 0o644)
			}
			if len(fails) > 0 {
				show := 10
				if v := os.Getenv("XO_SELFHOST_FAILS"); v != "" {
					fmt.Sscan(v, &show)
				}
				n := len(fails)
				if n > show {
					fails = append(fails[:show], fmt.Sprintf("... and %d more", n-show))
				}
				t.Fatalf("%d differences:\n%s", n, strings.Join(fails, "\n"))
			}
		})
	}
}

// runSSAStages is runSSA with --stages, except for the dirs in
// buildOnly (the build stage only).
func runSSAStages(t *testing.T, bin, root string, dirs []string, buildOnly map[string]bool, flags ...string) map[string]string {
	var staged, plain []string
	for _, d := range dirs {
		if buildOnly[d] {
			plain = append(plain, d)
		} else {
			staged = append(staged, d)
		}
	}
	out := runSSA(t, bin, root, staged, append([]string{"--stages"}, flags...)...)
	if len(plain) > 0 {
		for d, text := range runSSA(t, bin, root, plain, flags...) {
			out[d] = text
		}
	}
	return out
}

// goSSA is the recorded builder result for one run, cut into the pieces
// compareSSA compares (gogoldens_test.go): why there is no build stage
// (none: "rejected", "no main", or an error), else the build stage (the
// set "build types" of its type lines, the piece "build:<function>" of
// each function, closures renamed by canonClosures), and the stages of
// the passes (passStages) the recording printed (stages): the sorted
// lines "<stage>" of verify and mutants, the sorted type lines "<stage>
// types" and the functions "<stage>:<function>" of the others.
type goSSA struct {
	none   string
	stages map[string]bool
	p      *goldPieces
}

// goSSARaw is a builder's output for one run.
type goSSARaw struct {
	text   string
	none   string
	stages map[string]string
}

// ssaPieces cuts a builder's output into goSSA's pieces.
func ssaPieces(g goSSARaw) *goldPieces {
	return ssaPiecesSkip(g, nil)
}

// ssaPiecesSkip is ssaPieces with the closures of the functions in skip
// named as compareSSA names the Xo builder's (canonClosures).
func ssaPiecesSkip(g goSSARaw, skip map[string]string) *goldPieces {
	p := newPieces()
	if g.none != "" {
		p.setMeta("none", g.none)
		return p
	}
	types, funcs, _ := splitSSA(g.text)
	p.addSet("build types", types...)
	p.addFamily("build")
	for name, text := range canonClosures(funcs, skip) {
		p.addMember("build", name, text)
	}
	var have []string
	for _, stage := range passStages {
		text, ok := g.stages[stage]
		if !ok {
			continue
		}
		have = append(have, stage)
		if stage == "verify" || stage == "mutants" {
			p.addList(stage, nonEmptyLines(text))
			continue
		}
		types, funcs, _ := splitSSA(text)
		p.addList(stage+" types", types)
		p.addFamily(stage)
		for name, text := range canonClosures(funcs, nil) {
			p.addMember(stage, name, text)
		}
	}
	p.setMeta("stages", strings.Join(have, " "))
	return p
}

// goSSAOf is the run of its (replayed) pieces; without passes no stage
// after the build.
func goSSAOf(p *goldPieces, passes bool) goSSA {
	g := goSSA{p: p, none: p.m("none"), stages: map[string]bool{}}
	if p.missing {
		g.none = p.missingText()
	}
	if passes {
		for _, s := range strings.Fields(p.m("stages")) {
			g.stages[s] = true
		}
	}
	return g
}

// passStages are the stages after the build that the Xo builder ports
// (decision 0150 stage 8), compared on the runs both build completely.
var passStages = []string{"unwind", "landings", "cleanup", "rc", "verify", "mutants"}

// goSSABuild is the recorded SSA of dir checked as a native build does,
// from root, with every pass stage; got is the Xo builder's output.
func goSSABuild(root, dir, got string) goSSA { return goSSABuildOpts(root, dir, false, true, got) }

// testDirs are the dirs whose .xo files declare test blocks (a line
// starting with `test "`).
func testDirs(t testing.TB, root string, dirs []string) []string {
	var out []string
	for _, d := range dirs {
		ents, err := os.ReadDir(filepath.Join(root, d))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range ents {
			if !strings.HasSuffix(e.Name(), ".xo") {
				continue
			}
			src, _ := os.ReadFile(filepath.Join(root, d, e.Name()))
			if regexp.MustCompile(`(?m)^test "`).Match(src) {
				out = append(out, d)
				break
			}
		}
	}
	return out
}

// goSSABuildOpts is goSSABuild, of the test binary (`--test`, stage 7)
// when tests is set, and without the passStages unless passes is set.
// Recorded as the ssadump kind (gogoldens_test.go) with every pass stage,
// dropped here when passes is not set; got is the Xo builder's output for
// dir, which a recording run records (with every stage: a recording run
// takes the full tier, whose runs print them all).
func goSSABuildOpts(root, dir string, tests, passes bool, got string) goSSA {
	wd := goldensWD()
	cx := newGoldenCtx(wd, root, dir)
	o := fmt.Sprintf("tests=%v lineroot=%s", tests, cx.norm(root))
	key := []string{o, cx.dirKey(wd, dir, true)}
	flags := "--each --std=std --stages --wd=" + wd
	if tests {
		flags += " --test"
	}
	hint := goldHint("TestSelfhostSSA(Mutations)?", "ssa", flags+" "+dir, wd)
	p := recordedPieces("ssadump", dir+" "+o, hint, key, cx, func() *goldPieces { return xoSSAPieces(got) })
	return goSSAOf(p, passes)
}

// xoSSAPieces is the Xo builder's output for a run as the pieces a
// recording records: why there is no build stage (rejected, no main,
// unsupported: compareSSA counts those runs), else the build stage
// without the functions the builder skipped, and the stages after it.
func xoSSAPieces(got string) *goldPieces {
	head := firstLine(got)
	switch {
	case head == "== rejected":
		return ssaPieces(goSSARaw{none: "rejected"})
	case strings.Contains(head, "no `main`"):
		return ssaPieces(goSSARaw{none: "no main"})
	case strings.HasPrefix(head, "== unsupported "):
		return ssaPieces(goSSARaw{none: "unsupported: " + strings.TrimPrefix(head, "== unsupported ")})
	case head != "== stage build":
		return ssaPieces(goSSARaw{none: "error " + head})
	}
	body, skipped, _ := strings.Cut(strings.TrimPrefix(got, "== stage build\n"), "== skipped\n")
	g := goSSARaw{stages: map[string]string{}}
	parts := stageHead.Split(body, -1)
	for i, m := range stageHead.FindAllStringSubmatch(body, -1) {
		g.stages[m[1]] = parts[i+1]
	}
	g.text = parts[0]
	skip := map[string]string{}
	for _, l := range nonEmptyLines(skipped) {
		name, why, _ := strings.Cut(l, ": ")
		skip[name] = why
	}
	return ssaPiecesSkip(g, skip)
}

// runSSA runs the Xo builder on every dir (--each) from root, with flags
// (--stages, --std=DIR, --test), and splits its output by run.
func runSSA(t *testing.T, bin, root string, dirs []string, flags ...string) map[string]string {
	out := map[string]string{}
	const batch = 60
	for _, xb := range runXoBatches(t, bin, root, append([]string{"--each", "--wd=" + workDir(t)}, flags...), dirs, batch) {
		chunk := xb.paths
		// at the start of a line: the dump of compiler/ssa itself holds
		// the text in a constant
		parts := runHead.Split(xb.out, -1)
		if len(parts) != len(chunk)+1 {
			t.Fatalf("Xo builder printed %d runs for %d directories", len(parts)-1, len(chunk))
		}
		for j, d := range chunk {
			head := d + "\n"
			if !strings.HasPrefix(parts[j+1], head) {
				t.Fatalf("Xo builder output for %s starts with %q", d, firstLine(parts[j+1]))
			}
			out[d] = strings.TrimPrefix(parts[j+1], head)
		}
	}
	return out
}

// ssaStats counts what compareSSA compared.
// compareSSAAll is compareSSA on every dir (prefix names it in the
// failures), the dirs at the same time in slots of the pools (the
// comparison of the stages of a large run, closures renamed by regular
// expressions, takes seconds; one after another they took most of the
// merge tier's TestSelfhostSSA/go, 2026-10-12): the counts and the
// failures in the order of dirs.
func compareSSAAll(t testing.TB, prefix string, dirs []string, want map[string]goSSA, got map[string]string) (ssaStats, []string) {
	t.Helper()
	return compareAll(t, "compare "+prefix+"SSA", dirs, func(i int, st *ssaStats) []string {
		d := dirs[i]
		return compareSSA(prefix+d, want[d], got[d], st)
	}, (*ssaStats).add)
}

// add adds the counts of o to s.
func (s *ssaStats) add(o ssaStats) {
	s.runs += o.runs
	s.built += o.built
	s.rejected += o.rejected
	s.noMain += o.noMain
	s.unsupported += o.unsupported
	s.funcs += o.funcs
	s.same += o.same
	s.skipped += o.skipped
	s.missing += o.missing
	addCounts(&s.skips, o.skips)
	addCounts(&s.unsupportedWhy, o.unsupportedWhy)
	addCounts(&s.stageRuns, o.stageRuns)
	addCounts(&s.stageOff, o.stageOff)
	addCounts(&s.stageSame, o.stageSame)
}

type ssaStats struct {
	runs, built, rejected, noMain, unsupported int
	funcs, same, skipped, missing              int
	skips                                      map[string]int
	unsupportedWhy                             map[string]int
	// per stage after the build: runs compared, runs not compared (the
	// Xo builder skipped a function, or the recording reported errors),
	// and functions identical
	stageRuns, stageOff, stageSame map[string]int
}

func (s ssaStats) String() string {
	out := fmt.Sprintf("%d runs: %d built by both, %d rejected by both, %d without main, %d the Xo builder cannot build; "+
		"the recording's %d functions in the runs built: %d identical, %d skipped by the Xo builder, %d not declared by it (callees of skipped code)",
		s.runs, s.built, s.rejected, s.noMain, s.unsupported, s.funcs, s.same, s.skipped, s.missing)
	for _, st := range passStages {
		what := "functions"
		if st == "verify" || st == "mutants" {
			what = "lines"
		}
		out += fmt.Sprintf("; stage %s: %d runs compared (%d %s identical), %d not", st, s.stageRuns[st], s.stageSame[st], what, s.stageOff[st])
	}
	return out
}

// skipReport lists why functions and runs were not compared, most first.
func (s ssaStats) skipReport() string {
	var b strings.Builder
	b.WriteString(s.String() + "\n")
	for _, m := range []map[string]int{s.unsupportedWhy, s.skips} {
		var keys []string
		for k := range m {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return m[keys[i]] > m[keys[j]] || m[keys[i]] == m[keys[j]] && keys[i] < keys[j] })
		for _, k := range keys {
			fmt.Fprintf(&b, "%6d %s\n", m[k], k)
		}
	}
	return b.String()
}

// compareSSA compares one run and returns its differences.
func compareSSA(dir string, want goSSA, got string, st *ssaStats) []string {
	st.runs++
	if st.skips == nil {
		st.skips, st.unsupportedWhy = map[string]int{}, map[string]int{}
	}
	head := firstLine(got)
	switch {
	case want.none == "holes" || strings.HasPrefix(head, "== unsupported use go") || strings.HasPrefix(head, "== unsupported use c"):
		st.unsupported++
		st.unsupportedWhy["run: "+strings.TrimPrefix(head, "== ")]++
		return nil
	case want.none == "rejected":
		if head != "== rejected" && !strings.HasPrefix(head, "== unsupported ") {
			return []string{dir + ": the recording rejects it, the Xo checker does not: " + head}
		}
		st.rejected++
		return nil
	case head == "== rejected":
		return []string{dir + ": the Xo checker rejects it, the recording does not"}
	case want.none == "no main":
		if !strings.Contains(head, "no `main`") {
			return []string{dir + ": no main for the recording, the Xo builder prints " + head}
		}
		st.noMain++
		return nil
	case strings.HasPrefix(want.none, "unsupported: "):
		// recorded from the Xo builder, which could not build it
		st.unsupported++
		st.unsupportedWhy["run: "+strings.TrimPrefix(want.none, "unsupported: ")]++
		return nil
	case want.none != "":
		return []string{dir + ": the recording: " + want.none}
	case strings.HasPrefix(head, "== unsupported "):
		st.unsupported++
		st.unsupportedWhy["run: "+strings.TrimPrefix(head, "== unsupported ")]++
		return nil
	case head != "== stage build":
		return []string{dir + ": the Xo builder prints " + head}
	}
	st.built++
	body, skipped, _ := strings.Cut(strings.TrimPrefix(got, "== stage build\n"), "== skipped\n")
	// the stages of the passes follow the build stage
	xStages := map[string]string{}
	parts := stageHead.Split(body, -1)
	names := stageHead.FindAllStringSubmatch(body, -1)
	body = parts[0]
	for i, m := range names {
		xStages[m[1]] = parts[i+1]
	}
	xTypes, xFuncs, _ := splitSSA(body)
	skip := map[string]string{}
	for _, l := range nonEmptyLines(skipped) {
		name, why, _ := strings.Cut(l, ": ")
		skip[name] = why
	}
	xFuncs = canonClosures(xFuncs, skip)
	g := want.p.resolved("build", xFuncs, sortedKeys(skip)).resolvedSet("build types", xTypes)
	var fails []string
	for _, l := range xTypes {
		if !g.inSet("build types", l) {
			fails = append(fails, fmt.Sprintf("%s: the Xo builder's type line is not the recording's: %s", dir, l))
		}
	}
	for _, name := range g.names("build:") {
		st.funcs++
		x, ok := xFuncs[name]
		switch {
		case ok && g.eq("build:"+name, x):
			st.same++
		case ok:
			fails = append(fails, fmt.Sprintf("%s: %s differs: %s", dir, name, g.diff("build:"+name, x)))
		case skip[name] != "":
			st.skipped++
			st.skips[skip[name]]++
		default:
			st.missing++
		}
	}
	for name := range xFuncs {
		if !g.has("build:" + name) {
			fails = append(fails, fmt.Sprintf("%s: the Xo builder makes %s, the recording does not", dir, name))
		}
	}
	if st.stageRuns == nil {
		st.stageRuns, st.stageOff, st.stageSame = map[string]int{}, map[string]int{}, map[string]int{}
	}
	for _, stage := range passStages {
		gok := want.stages[stage]
		x, xok := xStages[stage]
		switch {
		case gok && xok:
			st.stageRuns[stage]++
			fails = append(fails, compareStage(dir, stage, g, x, st)...)
		case xok:
			fails = append(fails, fmt.Sprintf("%s: the Xo builder prints the stage %s, the recording does not", dir, stage))
		default:
			st.stageOff[stage]++
		}
	}
	sort.Strings(fails)
	return fails
}

// stageHead starts a stage after the build in the Xo builder's output.
var stageHead = regexp.MustCompile(`(?m)^== stage (\w+)\n`)

// compareStage compares the module after one pass, every function and
// type line (the Xo builder prints the stages of a run only when it
// built every function).
func compareStage(dir, stage string, want *goldPieces, got string, st *ssaStats) []string {
	if stage == "verify" || stage == "mutants" {
		// the verifier's lines (one per error, or per mutant: in the
		// order of the functions, which the builders make in different
		// orders), compared as sorted lists
		x := nonEmptyLines(got)
		if !want.eqList(stage, x) {
			return []string{fmt.Sprintf("%s: stage %s differs: %s", dir, stage, want.diffList(stage, x))}
		}
		st.stageSame[stage] += len(x)
		return nil
	}
	xTypes, xFuncs, _ := splitSSA(got)
	xFuncs = canonClosures(xFuncs, nil)
	want = want.resolved(stage, xFuncs, nil)
	var fails []string
	if !want.eqList(stage+" types", xTypes) {
		fails = append(fails, fmt.Sprintf("%s: stage %s: the type lines differ: %s", dir, stage, want.diffList(stage+" types", xTypes)))
	}
	for _, name := range want.names(stage + ":") {
		x, ok := xFuncs[name]
		switch {
		case !ok:
			fails = append(fails, fmt.Sprintf("%s: stage %s: the Xo builder has no %s", dir, stage, name))
		case !want.eq(stage+":"+name, x):
			fails = append(fails, fmt.Sprintf("%s: stage %s: %s differs: %s", dir, stage, name, want.diff(stage+":"+name, x)))
		default:
			st.stageSame[stage]++
		}
	}
	for name := range xFuncs {
		if !want.has(stage + ":" + name) {
			fails = append(fails, fmt.Sprintf("%s: stage %s: the Xo builder makes %s, the recording does not", dir, stage, name))
		}
	}
	return fails
}

// splitSSA splits a printed module into its type lines and its functions
// by name.
func splitSSA(text string) ([]string, map[string]string, []string) {
	types, funcs, order := splitModule(text)
	return types, funcs, order
}

var (
	closureName = regexp.MustCompile(`^(.*)\.fn(\d+)$`)
	runHead     = regexp.MustCompile(`(?m)^=== ssa `)
)

// canonClosures renames every closure `<parent>.fn<N>` (N counts the
// closures of the whole build) to `<parent>.fn#<rank of N among the
// closures of parent>`, in names and in the text of every function, so
// that closures are compared by their place in their function: the Go
// builder numbers them in the order it lowers the constructs of the
// whole program, and skipped constructs make the Xo builder's counter
// differ. skip, when given, is renamed in place.
func canonClosures(funcs map[string]string, skip map[string]string) map[string]string {
	names := map[string]bool{}
	for n := range funcs {
		names[n] = true
	}
	for n := range skip {
		names[n] = true
	}
	ref := regexp.MustCompile(`closure ([^ ,\n]+)`)
	for _, text := range funcs {
		for _, m := range ref.FindAllStringSubmatch(text, -1) {
			names[m[1]] = true
		}
	}
	byParent := map[string][]int{}
	for n := range names {
		if m := closureName.FindStringSubmatch(n); m != nil {
			k, _ := strconv.Atoi(m[2])
			byParent[m[1]] = append(byParent[m[1]], k)
		}
	}
	for p := range byParent {
		sort.Ints(byParent[p])
	}
	ren := map[string]string{}
	var canon func(n string) string
	canon = func(n string) string {
		if r, ok := ren[n]; ok {
			return r
		}
		m := closureName.FindStringSubmatch(n)
		if m == nil {
			return n
		}
		k, _ := strconv.Atoi(m[2])
		rank := sort.SearchInts(byParent[m[1]], k) + 1
		ren[n] = canon(m[1]) + ".fn#" + strconv.Itoa(rank)
		return ren[n]
	}
	for n := range names {
		canon(n)
	}
	if len(ren) == 0 {
		return funcs
	}
	rn := newRenamer(ren)
	out := map[string]string{}
	for n, text := range funcs {
		// two passes: adjacent names share the separator between them
		text = rn.pass(rn.pass(text))
		if r, ok := ren[n]; ok {
			n = r
		}
		out[n] = text
	}
	for n, why := range skip {
		if r, ok := ren[n]; ok && r != n {
			delete(skip, n)
			skip[r] = why
		}
	}
	return out
}

// renamer replaces the names of ren in a text as one pass of the
// regular expression `(^|[ (,])(names)([(, \n]|$)` (names longest first)
// does with ReplaceAllString, without the regular expression: its one
// alternation of every closure name ran over every function of every
// stage and took 50 s on the build stage of compiler/xo alone and most
// of TestSelfhostSSA in the full tier (2026-10-12). Names may hold the
// separators (generic instances: `f[A, B].fn1`).
type renamer struct {
	ren map[string]string
	// the lengths of the names by their first byte, longest first
	lens [256][]int
}

func newRenamer(ren map[string]string) *renamer {
	r := &renamer{}
	r.ren = ren
	seen := map[[2]int]bool{}
	for k := range ren {
		if k == "" || seen[[2]int{int(k[0]), len(k)}] {
			continue
		}
		seen[[2]int{int(k[0]), len(k)}] = true
		r.lens[k[0]] = append(r.lens[k[0]], len(k))
	}
	for i := range r.lens {
		sort.Sort(sort.Reverse(sort.IntSlice(r.lens[i])))
	}
	return r
}

// name is the longest name of ren at text[q:] followed by a separator
// or the end, or "".
func (r *renamer) name(text string, q int) string {
	if q >= len(text) {
		return ""
	}
	for _, l := range r.lens[text[q]] {
		e := q + l
		if e > len(text) {
			continue
		}
		if e < len(text) {
			switch text[e] {
			case '(', ',', ' ', '\n':
			default:
				continue
			}
		}
		if _, ok := r.ren[text[q:e]]; ok {
			return text[q:e]
		}
	}
	return ""
}

// pass is one ReplaceAllString of the regular expression: matches start
// leftmost, do not overlap, and take their trailing separator.
func (r *renamer) pass(text string) string {
	var b strings.Builder
	last := 0
	for p := 0; p <= len(text); {
		// (^|[ (,]): the empty start of the text first, then a separator
		var k string
		q := -1
		if p == 0 {
			if k = r.name(text, 0); k != "" {
				q = 0
			}
		}
		if q < 0 && p < len(text) && (text[p] == ' ' || text[p] == '(' || text[p] == ',') {
			if k = r.name(text, p+1); k != "" {
				q = p + 1
			}
		}
		if q < 0 {
			p++
			continue
		}
		b.WriteString(text[last:q])
		b.WriteString(r.ren[k])
		e := q + len(k)
		if e < len(text) {
			e++ // the trailing separator
		}
		b.WriteString(text[q+len(k) : e])
		last = e
		if e == p {
			// an empty match cannot happen: a name is not empty
			e++
		}
		p = e
	}
	if last == 0 {
		return text
	}
	b.WriteString(text[last:])
	return b.String()
}
