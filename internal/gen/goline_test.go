package gen_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/gen"
	"github.com/xo-lang/xo/internal/gotool"
)

// TestGoModLine: the generated go.mod carries Options.GoLine, and
// gotool.TestedGo when it is empty (decision 0176).
func TestGoModLine(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.xo"), []byte("fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n"), 0o644)
	res, err := check.Check([]string{dir})
	if err != nil || len(res.Diagnostics) > 0 {
		t.Fatalf("check: %v %v", err, res.Diagnostics)
	}
	for line, want := range map[string]string{"": gotool.TestedGo, "1.24": "1.24"} {
		prog, err := gen.Generate(res, gen.Options{Mode: gen.Build, GoLine: line})
		if err != nil {
			t.Fatal(err)
		}
		if mod := string(prog.Files["go.mod"]); !strings.Contains(mod, "\ngo "+want+"\n") {
			t.Errorf("GoLine %q: go.mod\n%s\nwant go %s", line, mod, want)
		}
	}
}
