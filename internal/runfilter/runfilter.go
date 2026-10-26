// Package runfilter reads go test's -run and -skip patterns, so a test
// that builds every golden program before running them as subtests can
// build only the programs whose subtests will run (`-run
// TestSelfhostWasm/var_patterns$` compiled all 300 goldens, about 14
// minutes, to check one; gotchas/selfhost-wasm-run-filter-builds-every-golden.md).
package runfilter

import (
	"flag"
	"regexp"
	"strings"
	"testing"
)

// Sub reports which subtests of the running top-level test t the
// command line selects: keep(name) is false for a subtest name that
// -run's second element cannot match or that -skip names, and filtered
// is true when the patterns name subtests at all (so a test can relax a
// check that only holds for the whole set). Patterns are split at `/`
// outside brackets and parentheses, as go test splits them.
func Sub(t testing.TB) (keep func(name string) bool, filtered bool) {
	run := split(value("test.run"))
	skip := split(value("test.skip"))
	top := t.Name()
	var runRE, skipRE *regexp.Regexp
	if len(run) >= 2 && matches(run[0], top) {
		runRE = compile(t, run[1])
	}
	if len(skip) >= 2 && matches(skip[0], top) {
		skipRE = compile(t, skip[1])
	}
	keep = func(name string) bool {
		name = rewrite(name)
		if runRE != nil && !runRE.MatchString(name) {
			return false
		}
		return skipRE == nil || !skipRE.MatchString(name)
	}
	return keep, runRE != nil || skipRE != nil
}

func value(name string) string {
	if f := flag.Lookup(name); f != nil {
		return f.Value.String()
	}
	return ""
}

func compile(t testing.TB, p string) *regexp.Regexp {
	re, err := regexp.Compile(p)
	if err != nil {
		t.Fatalf("runfilter: %q: %v", p, err)
	}
	return re
}

func matches(p, name string) bool {
	re, err := regexp.Compile(p)
	return err == nil && re.MatchString(name)
}

// split is testing's splitRegexp: the pattern's elements, one per test
// level, split at a `/` that is not inside [] or ().
func split(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	cs, cp, start := 0, 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[':
			cs++
		case ']':
			if cs > 0 {
				cs--
			}
		case '(':
			if cs == 0 {
				cp++
			}
		case ')':
			if cs == 0 && cp > 0 {
				cp--
			}
		case '\\':
			i++
		case '/':
			if cs == 0 && cp == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}

// rewrite is the subtest name t.Run gives name: spaces become `_`.
func rewrite(name string) string {
	return strings.ReplaceAll(name, " ", "_")
}
