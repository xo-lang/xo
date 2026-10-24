package llvm

import (
	"flag"
	"fmt"
	"os"
	"testing"
)

// goldenTimeoutAdvice explains why this run cannot finish and how to run it
// instead, or is "" when the run is fine.
//
// The package's goldens need 46+ minutes (each builds and links with clang),
// and go test's default timeout is 10m0s: a bare `go test ./internal/llvm`
// therefore dies at 600s with a FAIL that names no test, after ten minutes
// of work (2026-10-24: two agents in one evening). A -run filter (one
// golden, `-run 'TestGolden/NAME$'`) or a timeout of one's own avoids it,
// and -short skips most of the work already.
func goldenTimeoutAdvice(timeout, run string, short bool) string {
	if short || run != "" || timeout != "10m0s" {
		return ""
	}
	return "internal/llvm: the full package needs 46+ minutes and go test's default timeout is 10m0s, so this run would fail at 600s naming no test.\n" +
		"Run one golden: go test ./internal/llvm -run 'TestGolden/NAME$' -v (NAME is a directory under testdata/golden)\n" +
		"or the whole package: go test -timeout 90m ./internal/llvm\n" +
		"(knowledgebase/infrastructure/agent-tools.md, Testing: the internal/llvm timeout)"
}

// flagText is a test flag's value as the go test command line set it,
// "" when not given.
func flagText(name string) string {
	f := flag.Lookup(name)
	if f == nil {
		return ""
	}
	return f.Value.String()
}

// exitOnGoldenTimeout stops the run before it does any work when the flags
// say it would time out. The advice goes to stderr, beside the test output.
func exitOnGoldenTimeout() {
	if advice := goldenTimeoutAdvice(flagText("test.timeout"), flagText("test.run"), testing.Short()); advice != "" {
		fmt.Fprintln(os.Stderr, advice)
		os.Exit(1)
	}
}
