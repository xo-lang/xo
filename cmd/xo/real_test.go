package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/mod/modtest"
)

// TestTestReal checks core 9.2: a test that names effects (`test "..."
// uses env`) is skipped by `xo test` and runs with real capabilities
// under `xo test --real`, where `os.env` reads the real environment.
func TestTestReal(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds Go")
	}
	setenv(t, "XO_NO_SERVE", "1")
	dir := filepath.Join(t.TempDir(), "realcap")
	modtest.WriteFiles(t, dir, map[string]string{
		"lib.xo": "pub fn twice(n: Int) -> Int { n * 2 }\n",
		"lib_test.xo": "test \"pure\" { expect(twice(2)) == 4 }\n\n" +
			"test \"reads the real environment\" uses env {\n  expect(os.env.get(\"XO_AUDIT_REAL\")) == Some(\"yes\")\n}\n",
	})
	setenv(t, "XO_AUDIT_REAL", "yes")
	r := xo(t, "test", "-v", dir).want(0)
	if !strings.Contains(r.out+r.err, "reads the real environment") || !strings.Contains(r.out+r.err, "--real") {
		t.Errorf("without --real the test is not reported as skipped:\n%s\n%s", r.out, r.err)
	}
	r = xo(t, "test", "--real", "-v", "--json", dir).want(0)
	if !strings.Contains(r.out, "reads the real environment") || strings.Contains(r.out, `"skip"`) {
		t.Errorf("--real did not run the test:\n%s\n%s", r.out, r.err)
	}
	// it really reads the environment: another value fails it
	setenv(t, "XO_AUDIT_REAL", "no")
	r = xo(t, "test", "--real", dir).want(1)
	if !strings.Contains(r.out+r.err, "reads the real environment") {
		t.Errorf("failing real test:\n%s\n%s", r.out, r.err)
	}
}
