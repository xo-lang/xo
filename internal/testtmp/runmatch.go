package testtmp

import (
	"flag"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// RequireRunMatch exits 1 when -test.run names no test or example of the
// package (decision 0437): `go test -run=TestGoldenRuns` (a misspelt
// golden test name) printed "testing: warning: no tests to run" and
// passed, so a gate that ran nothing read as green
// (gotchas/go-run-matches-nothing.md). Called from a TestMain after
// flag.Parse. A run pattern of ^$ or a benchmark or fuzz run is left
// alone: those deliberately run no tests.
func RequireRunMatch(m *testing.M) {
	run := flagValue("test.run")
	if run == "" || run == "^$" || flagValue("test.bench") != "" || flagValue("test.fuzz") != "" || flagValue("test.list") != "" {
		return
	}
	top := strings.SplitN(run, "/", 2)[0]
	re, err := regexp.Compile(top)
	if err != nil {
		return // the testing package reports a bad pattern itself
	}
	// a -skip with a / skips subtests only (-skip A/B leaves A running),
	// so only a -skip of whole tests can leave a matched name unrun
	var skip *regexp.Regexp
	if s := flagValue("test.skip"); s != "" && !strings.Contains(s, "/") {
		skip, _ = regexp.Compile(strings.SplitN(s, "/", 2)[0])
	}
	var names []string
	v := reflect.ValueOf(m).Elem()
	for _, field := range []string{"tests", "examples"} {
		f := v.FieldByName(field)
		for i := 0; f.IsValid() && i < f.Len(); i++ {
			name := f.Index(i).FieldByName("Name").String()
			names = append(names, name)
			if re.MatchString(name) && (skip == nil || !skip.MatchString(name)) {
				return
			}
		}
	}
	if len(names) == 0 {
		return // the layout of testing.M changed: say nothing rather than fail every run
	}
	fmt.Fprintf(os.Stderr, "-run=%s matches no test in this package (%d tests); a misspelt test name runs nothing and would pass\n", run, len(names))
	os.Exit(1)
}

func flagValue(name string) string {
	if f := flag.Lookup(name); f != nil {
		return f.Value.String()
	}
	return ""
}
