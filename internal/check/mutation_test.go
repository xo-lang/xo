package check

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var mutations = flag.Bool("mutations", false, "run the corpus mutation test (slow)")

// corpusMutation finds sites in a source file and the text that replaces
// each site.
type corpusMutation struct {
	name    string
	re      *regexp.Regexp
	group   int // submatch replaced (0: whole match)
	replace string
	every   int // use every n-th site (1: all)
	tests   bool
}

var corpusMutations = []corpusMutation{
	{name: "remove a postfix ?", re: regexp.MustCompile(`\)(\?)[\s\).,]`), group: 1, every: 1, tests: true},
	{name: "delete one match arm", re: regexp.MustCompile(`\n[ \t]+[A-Z][A-Za-z]*(\([^\n]*\))? =>[^\n]*`), every: 1, tests: true},
	{name: "var to let", re: regexp.MustCompile(`\b(var) [a-z]`), group: 1, replace: "let", every: 1, tests: true},
	{name: "delete a numeric conversion", re: regexp.MustCompile(`\b(Float|Int)\(`), group: 1, every: 1, tests: true},
	{name: "delete a derive clause", re: regexp.MustCompile(` derive [A-Za-z, ]+`), every: 1, tests: true},
	{name: "typo in a member name", re: regexp.MustCompile(`\.([a-z_]{4,})[^\w(]`), group: 1, replace: "zzqq", every: 3, tests: true},
	{name: "delete a uses clause", re: regexp.MustCompile(`\n\s*uses [^\n{]*`), every: 1, tests: true},
}

// TestCorpusMutations plants one mutation at a time in a copy of each v0.3
// task and counts how often the diagnostics change. It is the check that a
// clean corpus run is not the checker skipping code (see
// knowledgebase/gotchas/checker-error-type-hides-bugs.md). Run with
// `go test ./internal/check -run TestCorpusMutations -mutations -v`.
func TestCorpusMutations(t *testing.T) {
	if !*mutations {
		t.Skip("run with -mutations")
	}
	tasks, err := os.ReadDir(corpusDir)
	if err != nil {
		t.Skip(err)
	}
	for _, mu := range corpusMutations {
		detected, total := 0, 0
		var missed []string
		for _, task := range tasks {
			if !task.IsDir() {
				continue
			}
			src := filepath.Join(corpusDir, task.Name())
			base := diagKey(t, src)
			files := xoFiles(t, src)
			for _, file := range files {
				data, _ := os.ReadFile(filepath.Join(src, file))
				text := string(data)
				locs := mu.re.FindAllStringSubmatchIndex(text, -1)
				for i, loc := range locs {
					if i%mu.every != 0 {
						continue
					}
					a, b := loc[2*mu.group], loc[2*mu.group+1]
					mutated := text[:a] + mu.replace + text[b:]
					dir := t.TempDir()
					copyTree(t, src, dir)
					if err := os.WriteFile(filepath.Join(dir, file), []byte(mutated), 0o644); err != nil {
						t.Fatal(err)
					}
					total++
					if diagKey(t, dir) != base {
						detected++
					} else {
						line := strings.Count(text[:a], "\n") + 1
						missed = append(missed, fmt.Sprintf("%s/%s:%d: %s", task.Name(), file, line, strings.TrimSpace(text[a:b])))
					}
				}
			}
		}
		t.Logf("%s: %d sites, %d detected", mu.name, total, detected)
		for _, m := range missed {
			t.Logf("  not detected: %s", m)
		}
	}
}

// diagKey runs the checker on dir and returns its diagnostics without paths.
func diagKey(t *testing.T, dir string) string {
	t.Helper()
	res, err := Check([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, d := range res.Diagnostics {
		keys = append(keys, d.Code+" "+d.Message)
	}
	sort.Strings(keys)
	return strings.Join(keys, "\n")
}

func xoFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".xo") {
			rel, _ := filepath.Rel(root, p)
			out = append(out, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}
