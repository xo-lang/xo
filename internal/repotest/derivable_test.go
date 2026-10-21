package repotest

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestDerivableTable: the derivable interfaces live in one table
// (compiler/check/derivables.xo). Every entry has its interface in the
// prelude and a mention in the spec, and no other compiler file keeps a
// list of three or more of the names, so a new derive is one entry.
func TestDerivableTable(t *testing.T) {
	tablePath := filepath.Join(root, "compiler", "check", "derivables.xo")
	table, err := os.ReadFile(tablePath)
	if err != nil {
		t.Fatal(err)
	}
	names := regexp.MustCompile(`Derivable\{name: "([A-Za-z]+)"`).FindAllStringSubmatch(string(table), -1)
	if len(names) < 8 {
		t.Fatalf("found %d entries in %s", len(names), tablePath)
	}
	prelude, err := os.ReadFile(filepath.Join(root, "std", "prelude.xo"))
	if err != nil {
		t.Fatal(err)
	}
	spec, err := os.ReadFile(filepath.Join(root, "spec", "core.md"))
	if err != nil {
		t.Fatal(err)
	}
	var list []string
	for _, m := range names {
		n := m[1]
		list = append(list, n)
		if !regexp.MustCompile(`(?m)^interface ` + n + ` \{`).Match(prelude) {
			t.Errorf("std/prelude.xo declares no `interface %s`", n)
		}
		if !strings.Contains(string(spec), "`"+n+"`") {
			t.Errorf("spec/core.md never mentions `%s`", n)
		}
	}
	quoted := regexp.MustCompile(`"([A-Z][A-Za-z]+)"`)
	err = filepath.Walk(filepath.Join(root, "compiler"), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".xo") || path == tablePath {
			return err
		}
		// explain_data is prose; habits maps Rust's own derive names
		if strings.HasSuffix(path, "_test.xo") || strings.HasSuffix(path, "explain_data.xo") || strings.HasSuffix(path, "parser/habits.xo") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(data), "\n") {
			seen := map[string]bool{}
			for _, m := range quoted.FindAllStringSubmatch(line, -1) {
				for _, n := range list {
					if m[1] == n {
						seen[n] = true
					}
				}
			}
			if len(seen) >= 3 {
				rel, _ := filepath.Rel(root, path)
				t.Errorf("%s:%d keeps a list of derivable names; read compiler/check/derivables.xo instead", rel, i+1)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
