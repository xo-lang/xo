package repotest

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tmpAllowed are the Xo sources that may name TMPDIR or a literal /tmp:
// hostpath.xo is the one temporary directory rule (TMPDIR, TEMP, TMP, then
// the OS's own), the native runtime's dump.xo reads the same variables
// itself (it cannot import the compiler), and the others use /tmp only as
// the last cache directory of a machine with no home (not a temporary
// directory).
var tmpAllowed = map[string]bool{
	"compiler/hostpath/hostpath.xo": true,
	"std/native/llrt/dump.xo":       true,
	"compiler/check/deps.xo":        true,
	"compiler/cli/modules/env.xo":   true,
	"compiler/cli/dev/bisect.xo":    true,
	"compiler/cli/dev/dev.xo":       true,
}

// TestNoHostTmpFallback keeps the field report W2/W3 bug from returning
// (2026-10-23: `TMPDIR ?? "/tmp"` in eight places, so a Windows machine
// wrote its fuzz regressions and test binaries nowhere): the compiler and
// the native runtime ask hostpath.temp_dir for the temporary directory.
func TestNoHostTmpFallback(t *testing.T) {
	for _, top := range []string{"compiler", filepath.Join("std", "native")} {
		err := filepath.WalkDir(filepath.Join(root, top), func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".xo") || strings.HasSuffix(p, "_test.xo") {
				return err
			}
			rel, _ := filepath.Rel(root, p)
			rel = filepath.ToSlash(rel)
			if tmpAllowed[rel] {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			for i, line := range strings.Split(string(b), "\n") {
				if strings.Contains(line, `"TMPDIR"`) || strings.Contains(line, `"/tmp"`) || strings.Contains(line, `"/tmp/`) {
					if strings.HasPrefix(strings.TrimSpace(line), "//") {
						continue
					}
					t.Errorf("%s:%d: names TMPDIR or /tmp; use hostpath.temp_dir(os)", rel, i+1)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
