package parser

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// corpusKnownFailures lists bench/paper/v0.2 files expected not to parse,
// with the reason. Empty: every v0.2 solution parses under the 0.3 grammar.
var corpusKnownFailures = map[string]string{}

func corpusFiles(t *testing.T, dir string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Ext(path) == ".xo" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Skipf("corpus %s not available: %v", dir, err)
	}
	sort.Strings(files)
	return files
}

// TestCorpusV02 parses the blind paper solutions written against spec 0.2.
// They are expected to parse under 0.3 (their `{x}` interpolations are now
// literal text).
func TestCorpusV02(t *testing.T) {
	files := corpusFiles(t, "../../bench/paper/v0.2")
	if len(files) == 0 {
		t.Skip("no v0.2 corpus files")
	}
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		_, ds := ParseFile(path, src, 0)
		rel, _ := filepath.Rel("../../bench/paper/v0.2", path)
		reason, known := corpusKnownFailures[rel]
		switch {
		case len(ds) == 0 && known:
			t.Errorf("%s: parses now; remove it from corpusKnownFailures", rel)
		case len(ds) == 0:
			t.Logf("PASS %s", rel)
		case known:
			t.Logf("KNOWN FAIL %s (%s)", rel, reason)
		default:
			t.Errorf("FAIL %s:\n%s", rel, humans(ds))
		}
	}
}

// TestCorpusV01 parses the 0.1 solutions for information only: they use old
// syntax, so failures are logged, never fatal.
func TestCorpusV01(t *testing.T) {
	files := corpusFiles(t, "../../bench/paper/v0.1")
	pass := 0
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		_, ds := ParseFile(path, src, 0)
		rel, _ := filepath.Rel("../../bench/paper/v0.1", path)
		if len(ds) == 0 {
			pass++
			t.Logf("PASS %s", rel)
			continue
		}
		t.Logf("FAIL %s (information only):\n%s", rel, humans(ds))
	}
	t.Logf("v0.1: %d of %d files parse", pass, len(files))
}
