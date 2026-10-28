package repotest

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBackendCacheEnvExplicit keeps the gate's cache-off trap from
// returning (gotchas/test-inherits-gate-cache-off.md; Lang 2's PDB tests
// and TestStdFactsVerify, 2026-10-28): scripts/gate.sh exports
// XO_BACKEND_CACHE=off, and a test whose builds inherit os.Environ() with
// only XO_BACKEND_CACHE_DIR set runs with the cache on alone and off in a
// gate. A Go test function that sets XO_BACKEND_CACHE_DIR in a build's
// environment also says whether the cache is on (XO_BACKEND_CACHE=on or
// off) in the same function.
func TestBackendCacheEnvExplicit(t *testing.T) {
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if n := d.Name(); n == ".git" || n == "runs" || n == "node_modules" || n == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		// one top-level function at a time: from a line starting "func "
		// to the next line that is "}"
		lines := strings.Split(string(b), "\n")
		start, dirLine, explicit := -1, -1, false
		for i, line := range lines {
			switch {
			case strings.HasPrefix(line, "func "):
				start, dirLine, explicit = i, -1, false
			case start >= 0 && line == "}":
				if dirLine >= 0 && !explicit {
					t.Errorf("%s:%d: a build sets XO_BACKEND_CACHE_DIR without XO_BACKEND_CACHE=on|off in the same function (a gate exports XO_BACKEND_CACHE=off; gotchas/test-inherits-gate-cache-off.md)", rel, dirLine+1)
				}
				start = -1
			case start >= 0:
				if strings.HasPrefix(strings.TrimSpace(line), "//") {
					continue
				}
				if strings.Contains(line, `XO_BACKEND_CACHE_DIR=`) && dirLine < 0 {
					dirLine = i
				}
				if strings.Contains(line, `XO_BACKEND_CACHE=`) {
					explicit = true
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
