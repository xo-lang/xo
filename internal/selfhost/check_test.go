package selfhost

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/gen"
	"github.com/xo-lang/xo/internal/parser"
)

// TestSelfhostChecker runs the Xo checker (selfhost/check), built with
// each backend, and the Go checker (internal/check) on every directory of
// the repository that holds .xo files, each checked as a run of its own
// like `xo check <dir>`, and compares the names, types, and sigs sections
// of `xo typedump` (decision 0141, stages 1 to 3):
//
//   - every run the Go checker accepts (no errors) has identical names,
//     types, and sigs sections (the effects of signatures only when
//     compareSigEffects is set), and the Xo checker reports nothing;
//   - every run the Go checker rejects only with diagnostics of the
//     stages done (stageDiag: names, types, Option and Result rules,
//     generics) gets exactly those diagnostics from the Xo checker.
//
// `use go` runs load the binding the Go checker wrote to the gobind cache,
// `use c` runs the one it wrote to the cbind cache (decision 0152); the
// runs with `use c` are compared a second time checked as a native build
// checks them (`--native`, check.Options.Native), where C interop is
// allowed. Runs the Xo checker cannot load (a binding missing from the
// cache, a module to fetch) and runs rejected for other reasons are
// counted, not compared; the unsupported ones are logged by reason. The
// coverage (runs matching / runs) is logged. The go backend only, except
// in the full tier (tier_test.go).
// XO_SELFHOST_KEEP=dir writes each run's class to dir/check-<backend>.txt.
func TestSelfhostChecker(t *testing.T) {
	backends := tierBackends(t)
	root := repoRoot(t)
	dirs := moduleDirs(t, root)
	// The Go checker prints paths relative to the working directory, as
	// the Xo checker run from root does.
	t.Chdir(root)
	setModEnv(t, root)
	// the `use c` runs again, checked as a native build checks them
	cDirs := useCDirs(t, root, dirs)
	want := map[string]*goRun{}
	wantNative := map[string]*goRun{}
	// the Go checker's results, while the subtests run the Xo checker
	wait := background(t, func() {
		wants := make([]*goRun, len(dirs))
		parallel(t, "Go checker", dirs, func(i int) { wants[i] = goCheck(dirs[i]) })
		for i, d := range dirs {
			want[d] = wants[i]
		}
		for _, d := range cDirs {
			wantNative[d] = goCheckOpts(d, check.Options{Native: true})
		}
	})
	bins := selfhostBinaries(t, "check", backends)
	for _, b := range backends {
		bin, ok := bins[b]
		if !ok {
			continue
		}
		t.Run(b, func(t *testing.T) {
			t.Parallel()
			got := runChecker(t, bin, root, dirs)
			wait()
			var st checkStats
			var fails []string
			var report strings.Builder
			codes := map[string]int{}
			why := map[string]int{}
			for _, d := range dirs {
				before := st
				msg := compareRun(d, want[d], got[d], &st)
				if msg != "" {
					fails = append(fails, msg)
				}
				if st.unsupported > before.unsupported {
					why[unsupportedReason(got[d])]++
				}
				if st.stagedOK > before.stagedOK {
					for _, l := range want[d].staged {
						codes[l[len(`{"code":"`):len(`{"code":"XO0000`)]]++
					}
				}
				fmt.Fprintf(&report, "%s %s %s %s\n", st.class(before), d, strings.Join(want[d].other, ","), firstLine(got[d]))
			}
			if keep := os.Getenv("XO_SELFHOST_KEEP"); keep != "" {
				os.MkdirAll(keep, 0o755)
				os.WriteFile(filepath.Join(keep, "check-"+b+".txt"), []byte(report.String()), 0o644)
			}
			t.Logf("%d runs: %d accepted by Go (%d match), %d rejected only with diagnostics of the stages done (%d match), %d rejected otherwise (%d with the same staged diagnostics), %d unsupported by the Xo checker, %d not loadable",
				len(dirs), st.accepted, st.acceptedOK, st.staged, st.stagedOK, st.otherRejected, st.otherSame, st.unsupported, st.errors)
			t.Logf("diagnostics of the matching rejected runs: %v", codes)
			t.Logf("unsupported runs by reason: %v", why)
			gotNative := runCheckerOpts(t, bin, root, cDirs, "--native")
			var nst checkStats
			for _, d := range cDirs {
				if msg := compareRun(d, wantNative[d], gotNative[d], &nst); msg != "" {
					fails = append(fails, "native: "+msg)
				}
			}
			t.Logf("%d `use c` runs checked as native builds: %d accepted by Go (%d match), %d rejected only with diagnostics of the stages done (%d match), %d rejected otherwise, %d unsupported by the Xo checker",
				len(cDirs), nst.accepted, nst.acceptedOK, nst.staged, nst.stagedOK, nst.otherRejected, nst.unsupported)
			if len(fails) > 0 {
				show := 10
				if v := os.Getenv("XO_SELFHOST_FAILS"); v != "" {
					fmt.Sscan(v, &show)
				}
				if len(fails) > show {
					fails = append(fails[:show], fmt.Sprintf("... and %d more", len(fails)-show))
				}
				t.Fatalf("%d of %d runs differ:\n%s", len(fails), len(dirs), strings.Join(fails, "\n"))
			}
		})
	}
}

type checkStats struct {
	accepted, acceptedOK, staged, stagedOK, otherRejected, otherSame, unsupported, errors int
}

// class names what compareRun counted for a run since before.
func (s checkStats) class(before checkStats) string {
	switch {
	case s.acceptedOK > before.acceptedOK:
		return "accepted-match"
	case s.accepted > before.accepted:
		return "accepted-differ"
	case s.stagedOK > before.stagedOK:
		return "rejected-match"
	case s.staged > before.staged:
		return "rejected-differ"
	case s.otherRejected > before.otherRejected:
		return "rejected-other"
	case s.unsupported > before.unsupported:
		return "unsupported"
	}
	return "error"
}

// goRun is the Go checker's result for one run.
type goRun struct {
	err  error
	dump string // the names, types, and sigs sections
	// accepted: no errors; onlyStaged: every error is a stageDiag.
	accepted, onlyStaged bool
	staged               []string // the stageDiag lines
	full                 []string // the staged diagnostics as full lines (notes, fixes)
	other                []string // the codes of the other errors
}

// goCheck checks dir with the Go checker.
func goCheck(dir string) *goRun { return goCheckOpts(dir, check.Options{}) }

// goCheckOpts checks dir with the Go checker and options (Native).
func goCheckOpts(dir string, opts check.Options) *goRun {
	res, err := check.CheckOpts([]string{dir}, opts)
	if err != nil {
		return &goRun{err: err}
	}
	r := &goRun{dump: res.Dump(check.Sections{Names: true, Types: true, Sigs: true}), accepted: true, onlyStaged: true}
	var staged []diag.Diagnostic
	parsed := map[string]map[string]bool{}
	for _, d := range res.Diagnostics {
		line := check.DiagLines([]diag.Diagnostic{d})[0]
		isStaged := stageDiag(line) || parseDiag(parsed, d.Span.File, line)
		if d.Severity == diag.Error {
			r.accepted = false
			r.onlyStaged = r.onlyStaged && isStaged
			if !isStaged {
				r.other = append(r.other, d.Code)
			}
		}
		if isStaged {
			staged = append(staged, d)
		}
	}
	r.staged = check.DiagLines(staged)
	r.full = check.FullLines(staged)
	return r
}

// parseDiag reports whether line is a diagnostic the Go parser gives file:
// the lexer's, the parser's soft errors (the habits of other languages,
// decision 0087), and its hard errors, which the Xo parser reproduces with
// the tree the Go parser recovers. parsed caches the parser diagnostics of
// each file.
func parseDiag(parsed map[string]map[string]bool, file, line string) bool {
	set, ok := parsed[file]
	if !ok {
		set = map[string]bool{}
		if src, err := os.ReadFile(file); err == nil && !strings.HasPrefix(file, "std/") {
			_, ds := parser.ParseFile(file, src, 0)
			for _, l := range check.DiagLines(ds) {
				set[l] = true
			}
		}
		parsed[file] = set
	}
	return set[line]
}

// moduleDirs lists every directory under root that holds .xo files,
// relative to root, skipping dot directories, runs/, output/, std/ (its
// files are signature mode declarations), and hatchesFixture (checked by
// TestSelfhostCheckerHatches in a copy).
func moduleDirs(t testing.TB, root string) []string {
	seen := map[string]bool{}
	var dirs []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() && p != root && (strings.HasPrefix(d.Name(), ".") || rel == "std" || rel == "runs" || rel == "output" || rel == hatchesFixture) {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(p, ".xo") {
			dir := filepath.ToSlash(filepath.Dir(rel))
			if !seen[dir] {
				seen[dir] = true
				dirs = append(dirs, dir)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(dirs)
	return dirs
}

// runChecker runs the Xo checker on every dir (--each) from root and
// splits its output by run.
func runChecker(t *testing.T, bin, root string, dirs []string) map[string]string {
	return runCheckerOpts(t, bin, root, dirs)
}

// runCheckerOpts is runChecker with more flags (--native).
func runCheckerOpts(t *testing.T, bin, root string, dirs []string, flags ...string) map[string]string {
	out := map[string]string{}
	const batch = 100
	for _, xb := range runXoBatches(t, bin, root, append([]string{"--each", "--std=std", "--wd=" + workDir(t)}, flags...), dirs, batch) {
		chunk := xb.paths
		parts := strings.Split(xb.out, "=== check ")
		if len(parts) != len(chunk)+1 {
			t.Fatalf("Xo checker printed %d runs for %d directories", len(parts)-1, len(chunk))
		}
		for j, d := range chunk {
			head := d + "\n"
			if !strings.HasPrefix(parts[j+1], head) {
				t.Fatalf("Xo checker output for %s starts with %q", d, firstLine(parts[j+1]))
			}
			out[d] = strings.TrimPrefix(parts[j+1], head)
		}
	}
	return out
}

// setModEnv makes module loading deterministic for both checkers
// (decision 0141, dependency rules): the module cache is the fixture cache
// internal/selfhost/testdata/modules/cache, nothing is fetched
// (XO_OFFLINE), and no XO_WORK file is forced.
func setModEnv(t *testing.T, root string) {
	cache, err := filepath.Abs(filepath.Join(root, "internal", "selfhost", "testdata", "modules", "cache"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("XO_MODCACHE", cache)
	t.Setenv("XO_OFFLINE", "1")
	t.Setenv("XO_WORK", "")
}

// workDir is the working directory as the Go checker sees it (os.Getwd,
// which prefers $PWD), passed to the Xo checker as --wd so the absolute
// paths both print agree.
func workDir(t testing.TB) string {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return wd
}

// useCDirs are the dirs with a `use c` line in one of their .xo files.
func useCDirs(t testing.TB, root string, dirs []string) []string {
	re := regexp.MustCompile(`(?m)^use c "`)
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
			if re.Match(src) {
				out = append(out, d)
				break
			}
		}
	}
	return out
}

// unsupportedReason is the kind of an `== unsupported <why>` line: its
// words up to the first path (`use go without a current binding of`,
// `use c without the binding of`, `fetching`).
func unsupportedReason(got string) string {
	why := strings.TrimPrefix(firstLine(got), "== unsupported ")
	for _, stop := range []string{" of ", " in ", " "} {
		if i := strings.Index(why, stop); i >= 0 {
			return why[:i]
		}
	}
	return why
}

func firstLine(s string) string {
	l, _, _ := strings.Cut(s, "\n")
	return l
}

// compareRun compares one run and returns a description of the
// difference, or "".
func compareRun(dir string, want *goRun, got string, st *checkStats) string {
	switch {
	case want.err != nil:
		st.errors++
		return ""
	case strings.HasPrefix(got, "== unsupported "):
		st.unsupported++
		return ""
	}
	gotDump, gotDiags, gotFull := splitDiags(got)
	if want.accepted {
		st.accepted++
		wantDump := want.dump
		if !compareSigEffects {
			gotDump, wantDump = stripSigEffects(gotDump), stripSigEffects(wantDump)
		}
		if gotDump != wantDump {
			return fmt.Sprintf("%s: dumps differ: %s", dir, firstDiff(gotDump, wantDump))
		}
		// an accepted run may hold staged warnings (XO0804)
		if strings.Join(gotDiags, "\n") != strings.Join(want.staged, "\n") {
			return fmt.Sprintf("%s: the Go checker accepts it with:\n%s\nthe Xo checker reports:\n%s", dir, strings.Join(want.staged, "\n"), strings.Join(gotDiags, "\n"))
		}
		if msg := compareFullLines(dir, gotFull, want.full); msg != "" {
			return msg
		}
		st.acceptedOK++
		return ""
	}
	if !want.onlyStaged {
		st.otherRejected++
		// not compared: the other errors may hide or cause staged ones;
		// counted when the Xo checker still reports exactly the staged
		// part of the Go diagnostics
		if len(want.staged) > 0 && strings.Join(gotDiags, "\n") == strings.Join(want.staged, "\n") {
			st.otherSame++
		}
		return ""
	}
	st.staged++
	if strings.Join(gotDiags, "\n") != strings.Join(want.staged, "\n") {
		return fmt.Sprintf("%s: diagnostics differ:\n  Go:\n    %s\n  Xo:\n    %s", dir, strings.Join(want.staged, "\n    "), strings.Join(gotDiags, "\n    "))
	}
	if msg := compareFullLines(dir, gotFull, want.full); msg != "" {
		return msg
	}
	st.stagedOK++
	return ""
}

// compareSigEffects compares the effects of the sigs section (stage 4);
// stage 3 compares the signatures without them.
var compareSigEffects = true

// stripSigEffects drops the effects (` uses ...`, the last one of a
// line) of the lines of every sigs section of dump.
func stripSigEffects(dump string) string {
	lines := strings.Split(dump, "\n")
	in := false
	for i, l := range lines {
		switch {
		case l == "-- sigs":
			in = true
		case strings.HasPrefix(l, "--") || strings.HasPrefix(l, "=="):
			in = false
		case in:
			if j := strings.LastIndex(l, " uses "); j >= 0 {
				lines[i] = l[:j]
			}
		}
	}
	return strings.Join(lines, "\n")
}

// compareFull compares the full section (stage 7: severity, notes, and
// fixes of every compared diagnostic).
var compareFull = true

// compareFullLines compares the full lines of a run and returns a
// description of the first difference, or "".
func compareFullLines(dir string, got, want []string) string {
	if !compareFull || strings.Join(got, "\n") == strings.Join(want, "\n") {
		return ""
	}
	return fmt.Sprintf("%s: notes or fixes differ: %s", dir, firstDiff(strings.Join(got, "\n"), strings.Join(want, "\n")))
}

// splitDiags cuts a dump into its names, types, and sigs part, its
// diagnostic lines, and its full lines.
func splitDiags(dump string) (string, []string, []string) {
	names, diags, _ := strings.Cut(dump, "== diags\n")
	diags, full, _ := strings.Cut(diags, "== full\n")
	return names, nonEmptyLines(diags), nonEmptyLines(full)
}

func nonEmptyLines(s string) []string {
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

// stageDiagRE matches the diagnostics the stages done reproduce
// (decision 0141): name resolution (XO0601 to XO0606, unused imports),
// the type diagnostics of stage 2 (core 2, 3: mismatches, arguments,
// operands, conversions, inference, the Option and Result rules,
// patterns that do not fit, loop labels, constants, unknown effects, and
// the task failure rules that decide error types), and generics of
// stage 3 (XO0402, XO0404 on constraints, comparability, keys, and
// derives), effects of stage 4 (XO0701, real-time XO1401 to XO1405),
// and value semantics of stage 5 (XO0501, XO0502, XO0503, XO0510,
// XO0520, the XO0804 warning, unused bindings XO0110 and XO0201), and
// exhaustiveness of stage 6 (XO0301, XO0303, the XO0305 warning), and
// the habits of stage 8 (naming XO0101, the habits of other languages
// XO0177 of the checker and the parser, time layouts XO0420), and module
// loading (xo.mod, xo.work, xo.lock, the module cache and vendor/: XO1301
// to XO1307, effect ceilings XO1309, deny rules XO1312).
var stageDiagRE = regexp.MustCompile("^" + `\{"code":"(` + strings.Join([]string{
	`(XO0202|XO0203|XO0204|XO0205|XO0206|XO0302|XO0304|XO0401|XO0403|XO0405|XO0406|XO0407|XO0408|XO0409|XO0410|XO0601|XO0602|XO0603|XO0604|XO0605|XO0606|XO0607|XO0702|XO0801|XO0802|XO0803)",.*`,
	// stage 3: generics
	`(XO0402|XO0404)",.*`,
	// stage 4: effects and real-time functions
	`(XO0701|XO1401|XO1402|XO1403|XO1404|XO1405)",.*`,
	// stage 5: value semantics and unused bindings
	`(XO0110|XO0201|XO0501|XO0502|XO0503|XO0510|XO0520|XO0804)",.*`,
	// stage 6: exhaustiveness
	`(XO0301|XO0303|XO0305)",.*`,
	// stage 8: naming, habits, time layouts
	`(XO0101|XO0177|XO0420)",.*`,
	// holes (XO0900, a warning), for `xo vet` and `xo1 check` (decision 0178
	// stage 2)
	`(XO0900)",.*`,
	// modules: xo.mod, xo.work, xo.lock, the module cache, vendor/,
	// dependency rules, deny rules, effect ceilings
	`(XO1301|XO1302|XO1303|XO1304|XO1305|XO1306|XO1307|XO1309|XO1312)",.*`,
	// interop: names a Go or C binding skipped, a C header that cannot
	// be bound, `use c` on the Go backend (decisions 0041, 0116, 0152)
	`(XO1102|XO1104|XO1105|XO1201)",.*`,
	// supply chain and editions: an old edition name, an unvouched
	// escape hatch, effects wider than the effect lock (decisions 0155,
	// 0156)
	`(XO1315|XO1316|XO1317)",.*`,
}, "|") + `)\}$`)

func stageDiag(line string) bool { return stageDiagRE.MatchString(line) }

// TestSelfhostCheckerXoTests runs the Xo tests of selfhost/check (`xo
// test`): path helpers, JSON escaping, and checks of small modules in the
// test's in memory file system.
func TestSelfhostCheckerXoTests(t *testing.T) {
	if selfhostTier(t) == tierShort {
		t.Skip("builds and runs the Xo test binary")
	}
	dir := filepath.Join("..", "..", "selfhost", "check")
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
		t.Fatalf("xo test selfhost/check: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "0 failed") {
		t.Fatalf("unexpected test output:\n%s", out)
	}
}
