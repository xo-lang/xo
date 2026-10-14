package selfhost

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestSelfhostChecker runs the Xo checker (selfhost/check), built with
// each backend, on every directory of the repository that holds .xo
// files, each checked as a run of its own like `xo check <dir>`, and
// compares the names, types, and sigs sections of its typedump and its
// diagnostics with the recording (decision 0141, stages 1 to 3; the Go
// checker's until decision 0215, gogoldens_test.go):
//
//   - every run the recording accepts (no errors) has identical names,
//     types, and sigs sections, and the same warnings;
//   - every run the recording rejects only with diagnostics of the
//     stages done (stageDiag) gets exactly those diagnostics.
//
// `use go` runs load the binding in the gobind cache, `use c` runs the
// one in the cbind cache (decision 0152); the runs with `use c` are
// compared a second time checked as a native build checks them
// (`--native`), where C interop is allowed. Runs the Xo checker cannot
// load (a binding missing from the cache, a module to fetch) and runs
// rejected for other reasons are counted, not compared; the unsupported
// ones are logged by reason. The coverage (runs matching / runs) is
// logged. The go backend only, except in the full tier (tier_test.go).
// XO_SELFHOST_KEEP=dir writes each run's class to dir/check-<backend>.txt.
func TestSelfhostChecker(t *testing.T) {
	backends := tierBackends(t)
	root := repoRoot(t)
	dirs := moduleDirs(t, root)
	// paths are relative to the working directory, as the Xo checker
	// run from root prints them
	t.Chdir(root)
	setModEnv(t, root)
	// the `use c` runs again, checked as a native build checks them
	cDirs := useCDirs(t, root, dirs)
	bins := selfhostBinaries(t, "check", backends)
	for _, b := range backends {
		bin, ok := bins[b]
		if !ok {
			continue
		}
		t.Run(b, func(t *testing.T) {
			t.Parallel()
			got := runChecker(t, bin, root, dirs)
			want := map[string]*goRun{}
			for _, d := range dirs {
				want[d] = goCheck(d, got[d])
			}
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
					for _, l := range want[d].stagedLines(got[d]) {
						codes[l[len(`{"code":"`):len(`{"code":"XO0000`)]]++
					}
				}
				fmt.Fprintf(&report, "%s %s %s %s\n", st.class(before), d, strings.Join(want[d].other, ","), firstLine(got[d]))
			}
			if keep := os.Getenv("XO_SELFHOST_KEEP"); keep != "" {
				os.MkdirAll(keep, 0o755)
				os.WriteFile(filepath.Join(keep, "check-"+b+".txt"), []byte(report.String()), 0o644)
			}
			t.Logf("%d runs: %d accepted by the recording (%d match), %d rejected only with diagnostics of the stages done (%d match), %d rejected otherwise (%d with the same staged diagnostics), %d unsupported by the Xo checker, %d not loadable",
				len(dirs), st.accepted, st.acceptedOK, st.staged, st.stagedOK, st.otherRejected, st.otherSame, st.unsupported, st.errors)
			t.Logf("diagnostics of the matching rejected runs: %v", codes)
			t.Logf("unsupported runs by reason: %v", why)
			gotNative := runCheckerOpts(t, bin, root, cDirs, "--native")
			var nst checkStats
			for _, d := range cDirs {
				if msg := compareRun(d, goCheckOpts(d, true, gotNative[d]), gotNative[d], &nst); msg != "" {
					fails = append(fails, "native: "+msg)
				}
			}
			t.Logf("%d `use c` runs checked as native builds: %d accepted by the recording (%d match), %d rejected only with diagnostics of the stages done (%d match), %d rejected otherwise, %d unsupported by the Xo checker",
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

// goRun is the recorded checker result of one run.
type goRun struct {
	err error
	// accepted: no errors; onlyStaged: every error is a stageDiag.
	accepted, onlyStaged bool
	nstaged              int      // the number of stageDiag lines
	other                []string // the codes of the other errors
	// p holds the pieces compared (gogoldens_test.go): "dump" (the
	// names, types, and sigs sections), "staged" (the stageDiag lines),
	// and "full" (the staged diagnostics as full lines: notes, fixes);
	// meta "run" the verdict, meta "error" err
	p *goldPieces

	// set by xoRunPieces only
	dump         string
	staged, full []string
}

// stagedLines is the recorded stageDiag lines of the run whose Xo
// output is got: the Xo checker's, valid when compareRun found them the
// same.
func (r *goRun) stagedLines(got string) []string {
	if t, ok := r.p.text("staged"); ok {
		return nonEmptyLines(t)
	}
	_, diags, _ := splitDiags(got)
	return diags
}

// goCheck is the recorded result of checking dir (the typedump kind,
// gogoldens_test.go); got is the Xo checker's output for it, which a
// recording run records.
func goCheck(dir, got string) *goRun { return goCheckOpts(dir, false, got) }

// goCheckOpts is goCheck of a native check (--native) when native is
// set.
func goCheckOpts(dir string, native bool, got string) *goRun {
	wd := goldensWD()
	cx := newGoldenCtx(wd, dir)
	o := fmt.Sprintf("native=%v", native)
	key := []string{o, cx.dirKey(wd, dir, native)}
	flag := ""
	if native {
		flag = "--native "
	}
	hint := goldHint("TestSelfhostChecker(Mutations|ModuleMutations|Hatches)?", "check",
		"--each --std=std "+flag+"--wd="+wd+" "+dir, wd)
	p := recordedPieces("typedump", dir+" "+o, hint, key, cx, func() *goldPieces { return xoRunPieces(got) })
	return goRunOf(p)
}

// xoRunPieces is the Xo checker's output for a run as the pieces a
// recording records: every diagnostic is a staged one (the Xo checker
// reports the diagnostics it reproduces), the run is accepted when no
// full line is an error; a run the Xo checker cannot load is an error,
// not compared.
func xoRunPieces(got string) *goldPieces {
	if strings.HasPrefix(got, "== unsupported ") {
		return (&goRun{err: errors.New(firstLine(got))}).pieces()
	}
	dump, diags, full := splitDiags(got)
	r := &goRun{dump: dump, accepted: true, onlyStaged: true, staged: diags, full: full}
	for _, l := range full {
		if strings.Contains(l, `"severity":"error"`) {
			r.accepted = false
		}
	}
	return r.pieces()
}

// pieces is the run's pieces, to record.
func (r *goRun) pieces() *goldPieces {
	p := newPieces()
	if r.err != nil {
		p.setMeta("error", r.err.Error())
		return p
	}
	p.setMeta("run", fmt.Sprintf("accepted=%v onlyStaged=%v staged=%d other=%s", r.accepted, r.onlyStaged, len(r.staged), strings.Join(r.other, ",")))
	p.add("dump", r.dump)
	p.add("staged", strings.Join(r.staged, "\n"))
	p.add("full", strings.Join(r.full, "\n"))
	return p
}

// goRunOf is the run of its (replayed) pieces.
func goRunOf(p *goldPieces) *goRun {
	r := &goRun{p: p}
	if p.missing {
		r.err = errors.New(p.missingText())
		return r
	}
	if e, ok := p.meta["error"]; ok {
		r.err = errors.New(e)
		return r
	}
	for _, f := range strings.Fields(p.m("run")) {
		k, v, _ := strings.Cut(f, "=")
		switch k {
		case "accepted":
			r.accepted = v == "true"
		case "onlyStaged":
			r.onlyStaged = v == "true"
		case "staged":
			r.nstaged, _ = strconv.Atoi(v)
		case "other":
			if v != "" {
				r.other = strings.Split(v, ",")
			}
		}
	}
	return r
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

// workDir is the working directory as the recording sees it (os.Getwd,
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
	p := want.p
	gotStaged := strings.Join(gotDiags, "\n")
	// wantStaged is the recorded side's staged lines for a message (replayed:
	// that the hash differs, and how to see them)
	wantStaged := func(sep string) string {
		if t, ok := p.text("staged"); ok {
			return strings.ReplaceAll(t, "\n", sep)
		}
		return p.diff("staged", gotStaged)
	}
	if want.accepted {
		st.accepted++
		same, diff := p.eq("dump", gotDump), func() string { return p.diff("dump", gotDump) }
		if t, ok := p.text("dump"); ok && !compareSigEffects {
			// live only: a replayed dump is compared with its effects
			g, w := stripSigEffects(gotDump), stripSigEffects(t)
			same, diff = g == w, func() string { return firstDiff(g, w) }
		}
		if !same {
			return fmt.Sprintf("%s: dumps differ: %s", dir, diff())
		}
		// an accepted run may hold staged warnings (XO0804)
		if !p.eq("staged", gotStaged) {
			return fmt.Sprintf("%s: the recording accepts it with:\n%s\nthe Xo checker reports:\n%s", dir, wantStaged("\n"), gotStaged)
		}
		if msg := compareFullLines(dir, gotFull, p); msg != "" {
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
		if want.nstaged > 0 && p.eq("staged", gotStaged) {
			st.otherSame++
		}
		return ""
	}
	st.staged++
	if !p.eq("staged", gotStaged) {
		return fmt.Sprintf("%s: diagnostics differ:\n  Go:\n    %s\n  Xo:\n    %s", dir, wantStaged("\n    "), strings.Join(gotDiags, "\n    "))
	}
	if msg := compareFullLines(dir, gotFull, p); msg != "" {
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
func compareFullLines(dir string, got []string, want *goldPieces) string {
	g := strings.Join(got, "\n")
	if !compareFull || want.eq("full", g) {
		return ""
	}
	return fmt.Sprintf("%s: notes or fixes differ: %s", dir, want.diff("full", g))
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
	runXoTests(t, "check")
}
