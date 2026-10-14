// Package repotest holds the checks of the repository's own files: the
// decision index, the spec's code tables against the code table xo
// reports from (selfhost/cli/core/explain_data.xo), and the golden
// scripts. They lived in internal/diag until decision 0215 deleted it
// with the Go implementation of the compiler; the package has no Go code.
package repotest

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const root = "../.."

// TestDecisionNumbersUnique keeps knowledgebase/decisions free of two
// decisions with one number (sessions working in parallel each took the
// next free number twice, 0132 and 0133 on 2026-10-10), and checks that
// the folder index lists every decision once.
func TestDecisionNumbersUnique(t *testing.T) {
	dir := filepath.Join(root, "knowledgebase", "decisions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	index, err := os.ReadFile(filepath.Join(dir, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	name := regexp.MustCompile(`^(\d{4})-[a-z0-9-]+\.md$`)
	byNum := map[string]string{}
	for _, e := range entries {
		if e.Name() == "README.md" {
			continue
		}
		m := name.FindStringSubmatch(e.Name())
		if m == nil {
			t.Errorf("%s: decision file names are NNNN-short-title.md", e.Name())
			continue
		}
		if prev, ok := byNum[m[1]]; ok {
			t.Errorf("decision number %s is used twice: %s and %s; renumber the newer one to the next free number and update its references", m[1], prev, e.Name())
		}
		byNum[m[1]] = e.Name()
		if n := strings.Count(string(index), "("+e.Name()+")"); n != 1 {
			t.Errorf("%s is linked %d times in decisions/README.md, want 1", e.Name(), n)
		}
	}
}

// TestScriptsReadWantBytes keeps the golden harness scripts reading
// want.txt without newline translation. Python's text mode open turns a
// carriage return into "\n", so a golden whose output holds a raw "\r"
// (show_table prints '\r') never matched the program's output
// (gotchas/python-text-mode-want-cr.md).
func TestScriptsReadWantBytes(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(root, "scripts", "*.py"))
	if err != nil {
		t.Fatal(err)
	}
	read := regexp.MustCompile(`open\([^\n]*'want\.txt'\)[^\n]*`)
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			m := read.FindString(line)
			if m != "" && !strings.Contains(m, "newline=''") && !strings.Contains(m, "'rb'") {
				t.Errorf("%s: %q reads want.txt in text mode, which turns \\r into \\n; pass newline='' (or 'rb')", filepath.Base(f), strings.TrimSpace(line))
			}
		}
	}
}

// explainRow is one code of selfhost/cli/core/explain_data.xo, the table
// `xo explain` prints and every diagnostic's severity comes from.
var explainRow = regexp.MustCompile(`(?m)^    "(XO\d{4})" => Some\(Expl\{title: "((?:[^"\\]|\\.)*)", severity: "(\w+)"`)

// explainCodes is code -> severity of explain_data.xo.
func explainCodes(t *testing.T) (map[string]string, string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "selfhost", "cli", "core", "explain_data.xo"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, m := range explainRow.FindAllStringSubmatch(string(data), -1) {
		if _, dup := out[m[1]]; dup {
			t.Errorf("explain_data.xo has %s twice", m[1])
		}
		out[m[1]] = m[3]
	}
	if len(out) < 100 {
		t.Fatalf("found only %d codes in explain_data.xo", len(out))
	}
	return out, string(data)
}

// specSection returns the text of the core.md section that starts with
// heading, up to the next `### ` or `## ` heading.
func specSection(t *testing.T, heading string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "spec", "core.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	i := strings.Index(s, "\n"+heading+"\n")
	if i < 0 {
		t.Fatalf("core.md has no heading %q", heading)
	}
	s = s[i+len(heading)+2:]
	if j := regexp.MustCompile(`\n##+ `).FindStringIndex(s); j != nil {
		s = s[:j[0]]
	}
	return s
}

// TestSpecListsEveryCode keeps the code table of core 10.6 equal to the
// code table xo reports from (explain_data.xo): every code listed once,
// with its severity, and no code in the spec the table does not have.
func TestSpecListsEveryCode(t *testing.T) {
	sec := specSection(t, "### 10.6 Diagnostic codes")
	row := regexp.MustCompile("(?m)^\\| `(XO\\d{4})` \\| (\\w+) \\| ([^|]+) \\|")
	listed := map[string]string{}
	for _, m := range row.FindAllStringSubmatch(sec, -1) {
		if _, dup := listed[m[1]]; dup {
			t.Errorf("core 10.6 lists %s twice", m[1])
		}
		listed[m[1]] = m[2]
	}
	codes, _ := explainCodes(t)
	for c, sev := range codes {
		got, ok := listed[c]
		if !ok {
			t.Errorf("%s is not listed in core 10.6", c)
			continue
		}
		if got != sev {
			t.Errorf("core 10.6 gives %s severity %s, xo reports %s", c, got, sev)
		}
		delete(listed, c)
	}
	for c := range listed {
		t.Errorf("core 10.6 lists %s, which explain_data.xo does not define", c)
	}
}

// TestExplanationsNoEmDash keeps the explanation texts free of em-dashes
// (the owner's writing rule).
func TestExplanationsNoEmDash(t *testing.T) {
	_, text := explainCodes(t)
	for i, l := range strings.Split(text, "\n") {
		if strings.ContainsRune(l, '—') {
			t.Errorf("explain_data.xo:%d contains an em-dash", i+1)
		}
	}
}

// TestSpecListsEveryFaultCode keeps the fault code table of core 10.6
// equal to the `XO-Fnnn` codes the runtimes (std/rt, std/llrt) raise.
func TestSpecListsEveryFaultCode(t *testing.T) {
	sec := specSection(t, "### 10.6 Diagnostic codes")
	listed := map[string]bool{}
	for _, m := range regexp.MustCompile("(?m)^\\| `(XO-F\\d{3})` \\|").FindAllStringSubmatch(sec, -1) {
		listed[m[1]] = true
	}
	used := map[string]bool{}
	fault := regexp.MustCompile(`XO-F\d{3}`)
	for _, dir := range []string{filepath.Join(root, "std", "rt"), filepath.Join(root, "std", "llrt")} {
		err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(p, "_test.go") || !(strings.HasSuffix(p, ".go") || strings.HasSuffix(p, ".c") || strings.HasSuffix(p, ".h")) {
				return nil
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			for _, c := range fault.FindAllString(string(data), -1) {
				used[c] = true
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(used) == 0 {
		t.Fatal("no fault codes found in the runtimes")
	}
	for c := range used {
		if !listed[c] {
			t.Errorf("the runtime raises %s, which core 10.6 does not list", c)
		}
	}
	for c := range listed {
		if !used[c] {
			t.Errorf("core 10.6 lists %s, which no runtime raises", c)
		}
	}
}
