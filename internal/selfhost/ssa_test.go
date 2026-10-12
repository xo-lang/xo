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

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/ssa"
)

// TestSelfhostSSA runs the Xo SSA builder (selfhost/ssa), built with each
// backend, and internal/ssa's builder on every directory of the
// repository that holds .xo files, each checked as a native build checks
// it (std/native included) and built from `main`, and compares the build
// stage of `xo ssadump` (decision 0150, stage 1) function by function:
//
//   - every function the Xo builder completes is identical to the Go
//     builder's of the same name (the closures of a function, named
//     `<function>.fn<N>` by a counter of the whole build, are compared
//     under their rank among the closures of that function);
//   - every type line the Xo builder prints is one the Go builder prints;
//   - a run the Go checker rejects is rejected by the Xo checker, and one
//     without `main` is not built by either.
//
// Functions the Xo builder skips (a construct of a later stage) or never
// declares (the callees of skipped code) are counted, not compared. The
// coverage is logged. The go backend only, except in the full tier
// (tier_test.go); -short also takes every 25th run. XO_SELFHOST_BACKEND
// names one backend.
func TestSelfhostSSA(t *testing.T) {
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
			if i%25 == 0 {
				some = append(some, d)
			}
		}
		dirs = some
	}
	// the merge tier compares only the build stage of the runs whose
	// passes take minutes (buildOnlyMerge)
	buildOnly := map[string]bool{}
	if selfhostTier(t) == tierMerge {
		for _, d := range buildOnlyMerge {
			buildOnly[d] = true
		}
	}
	bins := selfhostBinaries(t, "ssa", backends)
	t.Chdir(root)
	setModEnv(t, root)
	wants := make([]goSSA, len(dirs))
	parallel(len(dirs), func(i int) { wants[i] = goSSABuildOpts(root, dirs[i], false, !buildOnly[dirs[i]]) })
	want := map[string]goSSA{}
	for i, d := range dirs {
		want[d] = wants[i]
	}
	// the test binaries (stage 7) of the runs with test blocks
	tdirs := testDirs(t, root, dirs)
	twants := make([]goSSA, len(tdirs))
	parallel(len(tdirs), func(i int) { twants[i] = goSSABuildOpts(root, tdirs[i], true, !buildOnly[tdirs[i]]) })
	twant := map[string]goSSA{}
	for i, d := range tdirs {
		twant[d] = twants[i]
	}
	for _, b := range backends {
		bin, ok := bins[b]
		if !ok {
			continue
		}
		t.Run(b, func(t *testing.T) {
			t.Parallel()
			got := runSSAStages(t, bin, root, dirs, buildOnly, "--std=std")
			var st ssaStats
			var fails []string
			for _, d := range dirs {
				fails = append(fails, compareSSA(d, want[d], got[d], &st)...)
			}
			t.Logf("%s", st)
			tgot := runSSAStages(t, bin, root, tdirs, buildOnly, "--std=std", "--test")
			var tst ssaStats
			for _, d := range tdirs {
				fails = append(fails, compareSSA("test "+d, twant[d], tgot[d], &tst)...)
			}
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

// buildOnlyMerge are the runs whose passes the merge tier does not
// compare: about 110 of the 160 s the go backend's Xo builder spends in
// the passes of all runs (selfhost/ssa 69 s, selfhost/check 22 s,
// huge_frame 19 s; 2026-10-11), and as much again for the Go builder.
// Their build stage is still compared, and the full tier compares every
// stage of them.
var buildOnlyMerge = []string{
	"internal/arm64/testdata/run/huge_frame",
	"selfhost/check",
	"selfhost/ssa",
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

// goSSA is the Go builder's result for one run: the build stage, or why
// there is none ("rejected", "no main", or an error), and the stages of
// the passes (passStages) when the build had no errors.
type goSSA struct {
	text   string
	none   string
	stages map[string]string
}

// passStages are the stages after the build that the Xo builder ports
// (decision 0150 stage 8), compared on the runs both build completely.
var passStages = []string{"unwind", "landings", "cleanup", "rc", "verify", "mutants"}

// goSSABuild checks dir as a native build does and prints the build stage
// of its SSA, as `xo ssadump` does from root.
func goSSABuild(root, dir string) goSSA { return goSSABuildOpts(root, dir, false, true) }

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

// goSSABuildOpts is goSSABuild, of the test binary (`xo ssadump --test`,
// stage 7) when tests is set, and without the passStages unless passes
// is set.
func goSSABuildOpts(root, dir string, tests, passes bool) goSSA {
	res, ok, err := build.CheckNative([]string{dir}, false)
	if err != nil {
		return goSSA{none: "error " + err.Error()}
	}
	if !ok {
		// holes (XO0900) stop a build; the Xo checker does not report
		// them (decision 0141), so such a run is not compared
		holesOnly := true
		for _, d := range res.Diagnostics {
			if d.Severity == diag.Error && d.Code != "XO0900" {
				holesOnly = false
			}
		}
		if holesOnly {
			return goSSA{none: "holes"}
		}
		return goSSA{none: "rejected"}
	}
	want := map[string]bool{"build": true}
	if passes {
		for _, s := range passStages {
			want[s] = true
		}
	}
	stages, err := ssa.BuildStages(res, ssa.Options{LineRoot: root, Tests: tests}, want)
	if len(stages) == 0 {
		if err != nil && strings.Contains(err.Error(), "no `main`") {
			return goSSA{none: "no main"}
		}
		return goSSA{none: fmt.Sprintf("no build stage: %v", err)}
	}
	g := goSSA{text: stages[0].Text, stages: map[string]string{}}
	for _, st := range stages[1:] {
		g.stages[st.Name] = st.Text
	}
	return g
}

// runSSA runs the Xo builder on every dir (--each) from root, with flags
// (--stages, --std=DIR, --test), and splits its output by run.
func runSSA(t *testing.T, bin, root string, dirs []string, flags ...string) map[string]string {
	out := map[string]string{}
	const batch = 60
	texts := runXoBatches(t, bin, root, append([]string{"--each", "--wd=" + workDir(t)}, flags...), dirs, batch)
	for k, text := range texts {
		i := k * batch
		chunk := dirs[i:min(i+batch, len(dirs))]
		// at the start of a line: the dump of selfhost/ssa itself holds
		// the text in a constant
		parts := runHead.Split(text, -1)
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
type ssaStats struct {
	runs, built, rejected, noMain, unsupported int
	funcs, same, skipped, missing              int
	skips                                      map[string]int
	unsupportedWhy                             map[string]int
	// per stage after the build: runs compared, runs not compared (the
	// Xo builder skipped a function, or the Go builder reported errors),
	// and functions identical
	stageRuns, stageOff, stageSame map[string]int
}

func (s ssaStats) String() string {
	out := fmt.Sprintf("%d runs: %d built by both, %d rejected by both, %d without main, %d the Xo builder cannot build; "+
		"the Go builder's %d functions in the runs built: %d identical, %d skipped by the Xo builder, %d not declared by it (callees of skipped code)",
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
			return []string{dir + ": the Go checker rejects it, the Xo checker does not: " + head}
		}
		st.rejected++
		return nil
	case head == "== rejected":
		return []string{dir + ": the Xo checker rejects it, the Go checker does not"}
	case want.none == "no main":
		if !strings.Contains(head, "no `main`") {
			return []string{dir + ": no main for the Go builder, the Xo builder prints " + head}
		}
		st.noMain++
		return nil
	case want.none != "":
		return []string{dir + ": the Go builder: " + want.none}
	case strings.HasPrefix(head, "== unsupported "):
		st.unsupported++
		st.unsupportedWhy["run: "+strings.TrimPrefix(head, "== unsupported ")]++
		return nil
	case head != "== stage build":
		return []string{dir + ": the Xo builder prints " + head}
	}
	st.built++
	gTypes, gFuncs, _ := splitSSA(want.text)
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
	gFuncs = canonClosures(gFuncs, nil)
	xFuncs = canonClosures(xFuncs, skip)
	var fails []string
	gt := map[string]bool{}
	for _, l := range gTypes {
		gt[l] = true
	}
	for _, l := range xTypes {
		if !gt[l] {
			fails = append(fails, fmt.Sprintf("%s: the Xo builder's type line is not the Go builder's: %s", dir, l))
		}
	}
	for name, text := range gFuncs {
		st.funcs++
		x, ok := xFuncs[name]
		switch {
		case ok && x == text:
			st.same++
		case ok:
			fails = append(fails, fmt.Sprintf("%s: %s differs: %s", dir, name, firstDiff(x, text)))
		case skip[name] != "":
			st.skipped++
			st.skips[skip[name]]++
		default:
			st.missing++
		}
	}
	for name := range xFuncs {
		if _, ok := gFuncs[name]; !ok {
			fails = append(fails, fmt.Sprintf("%s: the Xo builder makes %s, the Go builder does not", dir, name))
		}
	}
	if st.stageRuns == nil {
		st.stageRuns, st.stageOff, st.stageSame = map[string]int{}, map[string]int{}, map[string]int{}
	}
	for _, stage := range passStages {
		g, gok := want.stages[stage]
		x, xok := xStages[stage]
		switch {
		case gok && xok:
			st.stageRuns[stage]++
			fails = append(fails, compareStage(dir, stage, g, x, st)...)
		case xok:
			fails = append(fails, fmt.Sprintf("%s: the Xo builder prints the stage %s, the Go builder does not", dir, stage))
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
func compareStage(dir, stage, want, got string, st *ssaStats) []string {
	if stage == "verify" || stage == "mutants" {
		// the verifier's lines (one per error, or per mutant: in the
		// order of the functions, which the builders make in different
		// orders), compared as sorted lists
		g, x := nonEmptyLines(want), nonEmptyLines(got)
		sort.Strings(g)
		sort.Strings(x)
		if strings.Join(g, "\n") != strings.Join(x, "\n") {
			return []string{fmt.Sprintf("%s: stage %s differs: %s", dir, stage, firstDiff(strings.Join(x, "\n"), strings.Join(g, "\n")))}
		}
		st.stageSame[stage] += len(g)
		return nil
	}
	gTypes, gFuncs, _ := splitSSA(want)
	xTypes, xFuncs, _ := splitSSA(got)
	gFuncs = canonClosures(gFuncs, nil)
	xFuncs = canonClosures(xFuncs, nil)
	var fails []string
	sort.Strings(gTypes)
	sort.Strings(xTypes)
	if strings.Join(gTypes, "\n") != strings.Join(xTypes, "\n") {
		fails = append(fails, fmt.Sprintf("%s: stage %s: the type lines differ", dir, stage))
	}
	for name, text := range gFuncs {
		x, ok := xFuncs[name]
		switch {
		case !ok:
			fails = append(fails, fmt.Sprintf("%s: stage %s: the Xo builder has no %s", dir, stage, name))
		case x != text:
			fails = append(fails, fmt.Sprintf("%s: stage %s: %s differs: %s", dir, stage, name, firstDiff(x, text)))
		default:
			st.stageSame[stage]++
		}
	}
	for name := range xFuncs {
		if _, ok := gFuncs[name]; !ok {
			fails = append(fails, fmt.Sprintf("%s: stage %s: the Xo builder makes %s, the Go builder does not", dir, stage, name))
		}
	}
	return fails
}

// splitSSA splits a printed module into its type lines and its functions
// by name.
func splitSSA(text string) ([]string, map[string]string, []string) {
	types, funcs, order := ssa.SplitModule(text)
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
	var keys []string
	for k := range ren {
		keys = append(keys, regexp.QuoteMeta(k))
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	pat := regexp.MustCompile(`(^|[ (,])(` + strings.Join(keys, "|") + `)([(, \n]|$)`)
	out := map[string]string{}
	for n, text := range funcs {
		// two passes: adjacent names share the separator between them
		for range 2 {
			text = pat.ReplaceAllStringFunc(text, func(s string) string {
				sm := pat.FindStringSubmatch(s)
				return sm[1] + ren[sm[2]] + sm[3]
			})
		}
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
