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
// Runs the Xo checker cannot load (`use go`, `use c`, a parse error) and
// runs rejected for other reasons are counted, not compared. The coverage
// (runs matching / runs) is logged. -short uses the go backend only.
// XO_SELFHOST_KEEP=dir writes each run's class to dir/check-<backend>.txt.
func TestSelfhostChecker(t *testing.T) {
	backends := []string{"go", "arm64", "llvm"}
	if testing.Short() {
		backends = []string{"go"}
	}
	root := repoRoot(t)
	dirs := moduleDirs(t, root)
	bins := map[string]string{}
	for _, b := range backends {
		if b != "go" {
			if why := nativeSkip(b); why != "" {
				t.Logf("%s: %s", b, why)
				continue
			}
		}
		bin, err := selfhostBinary("check", b)
		if err != nil {
			t.Fatalf("%s: %v", b, err)
		}
		bins[b] = bin
	}
	// The Go checker prints paths relative to the working directory, as
	// the Xo checker run from root does.
	t.Chdir(root)
	want := map[string]*goRun{}
	for _, d := range dirs {
		want[d] = goCheck(d)
	}
	for _, b := range backends {
		bin, ok := bins[b]
		if !ok {
			continue
		}
		t.Run(b, func(t *testing.T) {
			got := runChecker(t, bin, root, dirs)
			var st checkStats
			var fails []string
			var report strings.Builder
			codes := map[string]int{}
			for _, d := range dirs {
				before := st
				msg := compareRun(d, want[d], got[d], &st)
				if msg != "" {
					fails = append(fails, msg)
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
			if len(fails) > 0 {
				if len(fails) > 10 {
					fails = append(fails[:10], fmt.Sprintf("... and %d more", len(fails)-10))
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
	other                []string // the codes of the other errors
}

// goCheck checks dir with the Go checker.
func goCheck(dir string) *goRun {
	res, err := check.Check([]string{dir})
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
	return r
}

// parseDiag reports whether line is a diagnostic the Go parser gives file:
// the lexer's and the parser's soft errors (the habits of other languages,
// decision 0087), which the Xo parser reproduces with the tree the Go
// parser recovers. A run with a hard parse error is unsupported by the Xo
// checker. parsed caches the parser diagnostics of each file.
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
// relative to root, skipping dot directories, runs/, output/, and std/
// (its files are signature mode declarations).
func moduleDirs(t testing.TB, root string) []string {
	seen := map[string]bool{}
	var dirs []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() && p != root && (strings.HasPrefix(d.Name(), ".") || rel == "std" || rel == "runs" || rel == "output") {
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
	out := map[string]string{}
	const batch = 100
	for i := 0; i < len(dirs); i += batch {
		chunk := dirs[i:min(i+batch, len(dirs))]
		args := append([]string{"--each", "--std=std"}, chunk...)
		text := runXo(t, bin, root, args)
		parts := strings.Split(text, "=== check ")
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
	case strings.HasPrefix(got, "== unsupported parse error"):
		// the Xo parser stops at a file the Go parser rejects too
		if want.accepted {
			return fmt.Sprintf("%s: the Go checker accepts it, the Xo checker reports %s", dir, firstLine(got))
		}
		st.otherRejected++
		return ""
	case strings.HasPrefix(got, "== unsupported "):
		st.unsupported++
		return ""
	}
	gotDump, gotDiags := splitDiags(got)
	if want.accepted {
		st.accepted++
		wantDump := want.dump
		if !compareSigEffects {
			gotDump, wantDump = stripSigEffects(gotDump), stripSigEffects(wantDump)
		}
		if gotDump != wantDump {
			return fmt.Sprintf("%s: dumps differ: %s", dir, firstDiff(gotDump, wantDump))
		}
		if len(gotDiags) > 0 {
			return fmt.Sprintf("%s: the Go checker accepts it, the Xo checker reports:\n%s", dir, strings.Join(gotDiags, "\n"))
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

// splitDiags cuts a dump into its names, types, and sigs part and its
// diagnostic lines.
func splitDiags(dump string) (string, []string) {
	names, diags, _ := strings.Cut(dump, "== diags\n")
	var lines []string
	for _, l := range strings.Split(diags, "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}
	return names, lines
}

// stageDiagRE matches the diagnostics the stages done reproduce
// (decision 0141): name resolution (XO0601 to XO0606, unused imports),
// the type diagnostics of stage 2 (core 2, 3: mismatches, arguments,
// operands, conversions, inference, the Option and Result rules,
// patterns that do not fit, loop labels, constants, unknown effects, and
// the task failure rules that decide error types), and generics of
// stage 3 (XO0402, XO0404 on constraints, comparability, keys, and
// derives). Left to later stages: unused bindings (XO0110 on locals,
// XO0201 on bindings), value semantics (XO0501 to XO0520),
// exhaustiveness (XO0301, XO0303), naming (XO0101), and the habits of
// other languages that are not name or type errors (XO0177).
var stageDiagRE = regexp.MustCompile("^" + `\{"code":"(` + strings.Join([]string{
	`XO0110","[^}]*"message":"import ` + "`" + `[^` + "`" + `]+` + "`" + ` is never used"`,
	`XO0201","[^}]*"message":"result of [^"]*is not handled"`,
	`(XO0202|XO0203|XO0204|XO0205|XO0206|XO0302|XO0304|XO0401|XO0403|XO0405|XO0406|XO0407|XO0408|XO0409|XO0410|XO0601|XO0602|XO0603|XO0604|XO0605|XO0606|XO0607|XO0702|XO0801|XO0803)",.*`,
	// stage 3: generics
	`(XO0402|XO0404)",.*`,
	// stage 4: effects and real-time functions
	`(XO0701|XO1401|XO1402|XO1403|XO1404|XO1405)",.*`,
	`XO0502","[^}]*"message":"cannot assign to this expression[^"]*"`,
}, "|") + `)\}$`)

func stageDiag(line string) bool { return stageDiagRE.MatchString(line) }

// TestSelfhostCheckerXoTests runs the Xo tests of selfhost/check (`xo
// test`): path helpers, JSON escaping, and checks of small modules in the
// test's in memory file system.
func TestSelfhostCheckerXoTests(t *testing.T) {
	if testing.Short() {
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
