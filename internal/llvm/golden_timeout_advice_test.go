package llvm

import (
	"strings"
	"testing"
)

// TestGoldenTimeoutAdvice: a bare run under go test's default timeout is
// stopped before it does any work, and the advice names both ways to run
// the package; every other run is left alone.
func TestGoldenTimeoutAdvice(t *testing.T) {
	for _, c := range []struct {
		timeout, run string
		short        bool
		want         bool
	}{
		{"10m0s", "", false, true},                              // go test ./internal/llvm
		{"10m0s", "TestGolden/bytes_search_cut$", false, false}, // one golden
		{"90m0s", "", false, false},                             // -timeout 90m
		{"10m0s", "", true, false},                              // -short finishes
		{"", "", false, false},                                  // no timeout flag (a test binary run directly)
	} {
		got := goldenTimeoutAdvice(c.timeout, c.run, c.short)
		if (got != "") != c.want {
			t.Errorf("timeout %q run %q short %v: advice %q, want advice %v", c.timeout, c.run, c.short, got, c.want)
			continue
		}
		if !c.want {
			continue
		}
		for _, want := range []string{"46+ minutes", "10m0s", "-timeout 90m", "TestGolden/NAME$", "agent-tools.md"} {
			if !strings.Contains(got, want) {
				t.Errorf("advice lacks %q:\n%s", want, got)
			}
		}
	}
}
