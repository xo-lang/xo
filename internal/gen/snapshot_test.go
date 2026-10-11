package gen_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/gen"
)

// snapshotPrograms are the golden programs whose generated Go is kept in
// testdata/gosnap, one per main construct, so a review shows how a code
// generation change reads, not only that behavior held. Few on purpose:
// every lowering change rewrites them.
var snapshotPrograms = []string{
	"closures", "consts_newtypes", "enums", "error_frames", "generics",
	"interfaces", "loops", "option_result", "patterns", "scope_tasks", "structs",
}

// TestGeneratedGoSnapshot compares the Go generated for each snapshot
// program (its module's packages) with testdata/gosnap/NAME.go.golden;
// -golden-update rewrites them.
func TestGeneratedGoSnapshot(t *testing.T) {
	for _, name := range snapshotPrograms {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			res, prog, err := build.Generate([]string{filepath.Join("testdata", "golden", name)}, build.Options{Mode: gen.Build})
			if err != nil || prog == nil {
				t.Fatalf("generate: %v %v", err, res.Diagnostics)
			}
			var paths []string
			for p := range prog.Files {
				if strings.HasPrefix(p, "m/") && strings.HasSuffix(p, ".go") {
					paths = append(paths, p)
				}
			}
			sort.Strings(paths)
			var b strings.Builder
			for _, p := range paths {
				b.WriteString("// ==== " + p + "\n")
				b.Write(prog.Files[p])
			}
			path := filepath.Join("testdata", "gosnap", name+".go.golden")
			if *updateGolden {
				os.MkdirAll(filepath.Dir(path), 0o755)
				if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("missing snapshot (run with -golden-update): %v", err)
			}
			if b.String() != string(want) {
				t.Errorf("generated Go differs from %s (review the change, then rerun with -run TestGeneratedGoSnapshot -golden-update)", path)
			}
		})
	}
}
