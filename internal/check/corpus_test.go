package check

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/diag"
)

var corpusReport = flag.Bool("corpus-report", false, "rewrite the generated counts in bench/results/check-corpus-v0.4-spec.md")

const corpusDir = "../../bench/paper/v0.3"

// corpusResult is the report for the current spec draft (0.4); the v0.3
// corpus is checked against it. bench/results/check-corpus-v0.3.md keeps
// the results under spec 0.3.
const corpusResult = "../../bench/results/check-corpus-v0.4-spec.md"

// TestCorpusV03 checks every module of the blind paper solutions written
// against spec 0.3, under the current spec. Diagnostics are data, not
// failures: the test only logs them (and, with -corpus-report, rewrites the
// generated counts section of bench/results/check-corpus-v0.4-spec.md). It
// fails only if the checker cannot run.
func TestCorpusV03(t *testing.T) {
	tasks, err := os.ReadDir(corpusDir)
	if err != nil {
		t.Skipf("corpus not available: %v", err)
	}
	var all []diag.Diagnostic
	var mods []string
	for _, task := range tasks {
		if !task.IsDir() {
			continue
		}
		res, err := Check([]string{filepath.Join(corpusDir, task.Name())})
		if err != nil {
			t.Fatalf("%s: %v", task.Name(), err)
		}
		for _, m := range res.Modules {
			mods = append(mods, m.Path)
		}
		all = append(all, res.Diagnostics...)
	}
	// Modules reached through xo.mod are loaded by absolute path.
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	for i := range all {
		relPaths(&all[i], filepath.ToSlash(root)+"/")
		relPaths(&all[i], "../../")
		t.Log(diag.Human(all[i]))
	}
	report := corpusCounts(all, mods)
	t.Logf("\n%s", report)
	if *corpusReport {
		data, err := os.ReadFile(corpusResult)
		if err != nil {
			t.Fatal(err)
		}
		const begin, end = "<!-- generated:begin -->\n", "<!-- generated:end -->"
		s := string(data)
		i, j := strings.Index(s, begin), strings.Index(s, end)
		if i < 0 || j < i {
			t.Fatalf("%s has no generated section markers", corpusResult)
		}
		s = s[:i+len(begin)] + report + s[j:]
		if err := os.WriteFile(corpusResult, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// corpusCounts renders the per file counts by code and the totals.
func corpusCounts(ds []diag.Diagnostic, mods []string) string {
	perFile := map[string]map[string]int{}
	totals := map[string]int{}
	var codesSeen []string
	for _, d := range ds {
		if perFile[d.Span.File] == nil {
			perFile[d.Span.File] = map[string]int{}
		}
		perFile[d.Span.File][d.Code]++
		if totals[d.Code] == 0 {
			codesSeen = append(codesSeen, d.Code)
		}
		totals[d.Code]++
	}
	sort.Strings(codesSeen)
	var b strings.Builder
	fmt.Fprintf(&b, "Modules checked: %d. Diagnostics: %d.\n\n", len(mods), len(ds))
	b.WriteString("| Code | Count | Meaning |\n|---|---|---|\n")
	for _, c := range codesSeen {
		fmt.Fprintf(&b, "| %s | %d | %s |\n", c, totals[c], diag.Descriptions[diag.Code(c)])
	}
	b.WriteString("\n| File | Diagnostics by code |\n|---|---|\n")
	files := sortedKeys(perFile)
	for _, f := range files {
		var parts []string
		for _, c := range sortedKeys(perFile[f]) {
			parts = append(parts, fmt.Sprintf("%s x%d", c, perFile[f][c]))
		}
		fmt.Fprintf(&b, "| `%s` | %s |\n", f, strings.Join(parts, ", "))
	}
	b.WriteString("\n| Diagnostic |\n|---|\n")
	for _, d := range ds {
		fmt.Fprintf(&b, "| `%s:%d:%d` %s: %s |\n", d.Span.File, d.Span.Line, d.Span.Col, d.Code, strings.ReplaceAll(d.Message, "|", "\\|"))
	}
	return b.String()
}

// TestSpecAppendixA type checks the complete example of spec/core.md
// Appendix A. specAppendixKnown lists the diagnostics it is known to
// produce because the example itself breaks a rule of the spec.
func TestSpecAppendixA(t *testing.T) {
	data, err := os.ReadFile("../../spec/core.md")
	if err != nil {
		t.Skip(err)
	}
	s := string(data)
	i := strings.Index(s, "## Appendix A")
	if i < 0 {
		t.Fatal("no Appendix A")
	}
	parts := strings.SplitN(s[i:], "```", 3)
	if len(parts) < 3 {
		t.Fatal("no code block in Appendix A")
	}
	src := parts[1][strings.IndexByte(parts[1], '\n')+1:]
	ds := checkSrc(t, src)
	// Empty since spec 0.4 removed the unused `use std/json`.
	specAppendixKnown := map[string]string{}
	for _, d := range ds {
		key := d.Code + " " + d.Message
		if reason, ok := specAppendixKnown[key]; ok {
			t.Logf("known: %s (%s)", key, reason)
			delete(specAppendixKnown, key)
			continue
		}
		t.Errorf("unexpected: %s", diag.Human(d))
	}
	for k := range specAppendixKnown {
		t.Errorf("known diagnostic no longer produced, update specAppendixKnown: %s", k)
	}
}

// TestNoPanicOnBrokenInput checks truncated programs and the older corpora
// (written for spec 0.1 and 0.2) without panicking; diagnostics are
// expected and not inspected.
func TestNoPanicOnBrokenInput(t *testing.T) {
	srcs := []string{plantedBase, elisionBase + groupsSample}
	for _, full := range srcs {
		for n := 0; n < len(full); n += 41 {
			src := full[:n]
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("panic on input truncated at %d: %v\n%s", n, r, src)
					}
				}()
				checkSrc(t, src)
			}()
		}
	}
	for _, dir := range []string{"../../bench/paper/v0.1", "../../bench/paper/v0.2"} {
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		res, err := Check([]string{dir})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: %d diagnostics", dir, len(res.Diagnostics))
	}
}

// groupsSample exercises the spec 0.4 syntax and rules (task groups, `is`,
// tuple fields, local const, ignore_err) for the truncation test.
const groupsSample = `
pub struct Svc {
  cache: Cache[Int, Str]
  tasks: task.Group
}

fn (s: Svc) warm(ids: List[Int]) -> Int {
  const LIMIT = 8
  var hits = (0, 0)
  for id in ids {
    if s.cache.get(id) is Ok(_) { hits.0 += 1 } else { hits.1 += 1 }
    s.tasks.spawn_background(fn() { s.cache.get(id).ignore_err("warming is best effort") })
  }
  let _ = s.cache.get(LIMIT)
  hits.0 * 1s
}
`
