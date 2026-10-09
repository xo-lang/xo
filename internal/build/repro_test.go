package build

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/xo-lang/xo/internal/gen"
)

// TestReproducibleBuild checks core 12.1: the same source (in the same
// directory) built twice, from two empty build directories, gives byte
// identical binaries, for the host and for a cross target.
func TestReproducibleBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("builds Go twice")
	}
	src := filepath.Join(t.TempDir(), "hello")
	os.MkdirAll(src, 0o755)
	os.WriteFile(filepath.Join(src, "main.xo"), []byte("fn main(os: Os) {\n  let xs = [3, 1, 2].sort()\n  os.stdio.println(\"hi ${xs.len()}\")\n}\n"), 0o644)
	for _, tg := range []Target{Host(), {"linux", "amd64"}} {
		var bins [][]byte
		for i := 0; i < 2; i++ {
			t.Setenv("XO_BUILD_DIR", t.TempDir())
			out := filepath.Join(t.TempDir(), "hello")
			res, err := Build([]string{src}, Options{Mode: gen.Build, Targets: []Target{tg}, Out: out})
			if err != nil || res.Failed {
				t.Fatalf("build %v: %v %+v", tg, err, res)
			}
			data, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			bins = append(bins, data)
		}
		if !bytes.Equal(bins[0], bins[1]) {
			t.Errorf("%v: two builds of the same source differ (%d and %d bytes)", tg, len(bins[0]), len(bins[1]))
		}
	}
}
