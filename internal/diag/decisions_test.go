package diag

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestDecisionNumbersUnique keeps knowledgebase/decisions free of two
// decisions with one number (sessions working in parallel each took the
// next free number twice, 0132 and 0133 on 2026-10-10), and checks that
// the folder index lists every decision once. It lives here because diag
// already reads the repo's docs (spec_test.go).
func TestDecisionNumbersUnique(t *testing.T) {
	const dir = "../../knowledgebase/decisions"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	index, err := os.ReadFile(dir + "/README.md")
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
