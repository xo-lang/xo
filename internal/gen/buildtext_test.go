package gen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestBuildFailuresShowOutput: a build failure message prints the build's
// output when it has no diagnostics (#528: TestStackSmallProperty/17 on
// host 2 said "the Go build failed: <nil>" and nothing else, with the host
// at 20 of 24 GB: diagText printed only diagnostics, and a toolchain
// killed for memory leaves none).
func TestBuildFailuresShowOutput(t *testing.T) {
	res := &xotool.Result{Failed: true, Output: "go build: signal: killed\n"}
	if got := buildText(res); !strings.Contains(got, "signal: killed") {
		t.Errorf("buildText = %q, want the build output", got)
	}
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f == "buildtext_test.go" {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			if strings.Contains(line, "build failed") && strings.Contains(line, "diagText(") {
				t.Errorf("%s:%d: a build failure printed with diagText (diagnostics only); use buildText", f, i+1)
			}
		}
	}
}
