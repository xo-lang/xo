package selfhost

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/diag"
)

// TestSelfhostCheckerNames runs the Xo checker (selfhost/check), built
// with each backend, and the Go checker (internal/check) on every
// directory of the repository that holds .xo files, each checked as a run
// of its own like `xo check <dir>`, and compares the names section of
// `xo typedump` (decision 0139):
//
//   - every run the Go checker accepts (no errors) has an identical names
//     section, and the Xo checker reports nothing;
//   - every run the Go checker rejects only with name resolution errors
//     (nameDiag) gets exactly those diagnostics from the Xo checker.
//
// Runs the Xo checker cannot load (`use go`, `use c`, a parse error) and
// runs rejected for other reasons are counted, not compared. The coverage
// (runs matching / runs) is logged. -short uses the go backend only.
// XO_SELFHOST_KEEP=dir writes each run's class to dir/check-<backend>.txt.
func TestSelfhostCheckerNames(t *testing.T) {
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
			for _, d := range dirs {
				before := st
				msg := compareRun(d, want[d], got[d], &st)
				if msg != "" {
					fails = append(fails, msg)
				}
				fmt.Fprintf(&report, "%s %s %s\n", st.class(before), d, firstLine(got[d]))
			}
			if keep := os.Getenv("XO_SELFHOST_KEEP"); keep != "" {
				os.MkdirAll(keep, 0o755)
				os.WriteFile(filepath.Join(keep, "check-"+b+".txt"), []byte(report.String()), 0o644)
			}
			t.Logf("%d runs: %d accepted by Go (%d match), %d rejected only for names (%d match), %d rejected otherwise, %d unsupported by the Xo checker, %d not loadable",
				len(dirs), st.accepted, st.acceptedOK, st.nameRejected, st.nameRejectedOK, st.otherRejected, st.unsupported, st.errors)
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
	accepted, acceptedOK, nameRejected, nameRejectedOK, otherRejected, unsupported, errors int
}

// class names what compareRun counted for a run since before.
func (s checkStats) class(before checkStats) string {
	switch {
	case s.acceptedOK > before.acceptedOK:
		return "accepted-match"
	case s.accepted > before.accepted:
		return "accepted-differ"
	case s.nameRejectedOK > before.nameRejectedOK:
		return "names-rejected-match"
	case s.nameRejected > before.nameRejected:
		return "names-rejected-differ"
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
	dump string // the names section
	// accepted: no errors; onlyNames: every error is a nameDiag.
	accepted, onlyNames bool
	named               []string // the nameDiag lines
}

// goCheck checks dir with the Go checker.
func goCheck(dir string) *goRun {
	res, err := check.Check([]string{dir})
	if err != nil {
		return &goRun{err: err}
	}
	r := &goRun{dump: res.Dump(check.Sections{Names: true}), accepted: true, onlyNames: true}
	var named []diag.Diagnostic
	for _, d := range res.Diagnostics {
		isName := nameDiag(check.DiagLines([]diag.Diagnostic{d})[0])
		if d.Severity == diag.Error {
			r.accepted = false
			r.onlyNames = r.onlyNames && isName
		}
		if isName {
			named = append(named, d)
		}
	}
	r.named = check.DiagLines(named)
	return r
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
	gotNames, gotDiags := splitDiags(got)
	if want.accepted {
		st.accepted++
		if gotNames != want.dump {
			return fmt.Sprintf("%s: names differ: %s", dir, firstDiff(gotNames, want.dump))
		}
		if len(gotDiags) > 0 {
			return fmt.Sprintf("%s: the Go checker accepts it, the Xo checker reports:\n%s", dir, strings.Join(gotDiags, "\n"))
		}
		st.acceptedOK++
		return ""
	}
	if !want.onlyNames {
		st.otherRejected++
		return ""
	}
	st.nameRejected++
	if strings.Join(gotDiags, "\n") != strings.Join(want.named, "\n") {
		return fmt.Sprintf("%s: name diagnostics differ:\n  Go:\n    %s\n  Xo:\n    %s", dir, strings.Join(want.named, "\n    "), strings.Join(gotDiags, "\n    "))
	}
	st.nameRejectedOK++
	return ""
}

// splitDiags cuts a dump into its names part and its diagnostic lines.
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

// nameDiagRE matches the diagnostics of name resolution that the names
// stage reproduces (decision 0139). A bare UpperCamel name may be a
// variant of the expected enum, so its XO0601 belongs to the types stage.
var nameDiagRE = regexp.MustCompile("^" + `\{"code":"(XO0601|XO0603|XO0604|XO0605)",.*"message":"(` + strings.Join([]string{
	"(name|function) `[a-z_][A-Za-z0-9_]*` not found",
	"(type|module) `[^`]+` not found",
	"module `[^`]+` has no (member|type) `[^`]+`",
	"type path `[^`]+` has too many segments",
	"derivable interface `[^`]+` not found",
	"`(result|old)` is only defined inside an `ensures` clause \\(core 9.1\\)",
	"import cycle: .*",
	".*is not `pub` in module .*",
	".*(already declared|declared twice|already imported|is predeclared|both a field and a method).*",
}, "|") + `)"\}$`)

func nameDiag(line string) bool { return nameDiagRE.MatchString(line) }
