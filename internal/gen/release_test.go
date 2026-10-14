package gen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestGoldenRelease builds every program under testdata/release twice,
// as a debug build (want.debug.txt) and as a release build, `xo build
// --release` (want.release.txt), and compares like TestGolden; with
// want.release_noargs.txt also as `xo build --release --error-args=false`.
// The files show what a release build changes (core 8): contracts marked
// `requires(debug)` or `ensures(debug)` are compiled out, and error frames
// drop parameter values only on request (decision 0092).
func TestGoldenRelease(t *testing.T) {
	dirs, err := filepath.Glob(filepath.Join("testdata", "release", "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) == 0 {
		t.Fatal("no release programs")
	}
	variants := []struct {
		want string
		opts xotool.Options
	}{
		{"want.debug.txt", xotool.Options{}},
		{"want.release.txt", xotool.Options{Release: true}},
		{"want.release_noargs.txt", xotool.Options{Release: true, NoErrorArgs: true}},
	}
	for _, dir := range dirs {
		dir := dir
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Parallel()
			for _, v := range variants {
				wantPath := filepath.Join(dir, v.want)
				want, err := os.ReadFile(wantPath)
				if os.IsNotExist(err) {
					// -golden-update rewrites existing files only
					continue
				}
				got, _ := buildAndRunOpts(t, dir, v.opts)
				got.stderr = stableStderr(got.stderr)
				if *updateGolden {
					if err := os.WriteFile(wantPath, []byte(got.String()), 0o644); err != nil {
						t.Fatal(err)
					}
					continue
				}
				if gs, ws := dropGoFrames(got.String()), dropGoFrames(string(want)); gs != ws {
					t.Errorf("%s: output differs\n--- got\n%s\n--- want\n%s", v.want, got.String(), want)
				}
			}
		})
	}
}
