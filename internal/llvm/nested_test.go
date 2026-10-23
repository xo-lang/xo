package llvm

import (
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestNestedUpdateInPlace requires the updates of collections inside
// list elements of the golden program nested_update to happen in place
// on the native backends (decision 0169): the program makes 159
// allocations, where copying the inner list on every update (before
// the decision) made 2,162. The golden test checks the output.
func TestNestedUpdateInPlace(t *testing.T) {
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found: the LLVM backend tests need it")
	}
	if testing.Short() {
		t.Skip("builds a program with each native backend")
	}
	dir := filepath.Join("testdata", "golden", "nested_update")
	backends := []string{xotool.BackendLLVM}
	if runtime.GOARCH == "arm64" && (runtime.GOOS == "darwin" || runtime.GOOS == "linux") {
		backends = append(backends, xotool.BackendArm64)
	}
	for _, be := range backends {
		res, err := xotool.Build([]string{dir}, xotool.Options{Backend: be})
		if err != nil || res.Failed {
			t.Fatalf("build (%s): %v %v", be, err, res)
		}
		line := leakCheck(t, res.Binaries[xotool.Host()], dir)
		m := leakRE.FindStringSubmatch(line)
		total, _ := strconv.Atoi(m[2])
		if total > 400 {
			t.Errorf("%s: %d allocations, want at most 400 (the inner lists copied?)", be, total)
		}
	}
}

// TestMapUpdateInPlace requires Map.update to change a map's value in
// place on the native backends: the closure got the value while the map
// still held it, so a push inside it copied the list on every call
// (2026-10-23, the checker's flow events: 98 GB allocated for a 32,768
// statement function). The golden pushes 20,000 values into four lists.
func TestMapUpdateInPlace(t *testing.T) {
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found: the LLVM backend tests need it")
	}
	if testing.Short() {
		t.Skip("builds a program with each native backend")
	}
	dir := filepath.Join("testdata", "golden", "map_update_inplace")
	backends := []string{xotool.BackendLLVM}
	if runtime.GOARCH == "arm64" && (runtime.GOOS == "darwin" || runtime.GOOS == "linux") {
		backends = append(backends, xotool.BackendArm64)
	}
	for _, be := range backends {
		res, err := xotool.Build([]string{dir}, xotool.Options{Backend: be})
		if err != nil || res.Failed {
			t.Fatalf("build (%s): %v %v", be, err, res)
		}
		line := leakCheck(t, res.Binaries[xotool.Host()], dir)
		m := leakRE.FindStringSubmatch(line)
		total, _ := strconv.Atoi(m[2])
		if total > 400 {
			t.Errorf("%s: %d allocations, want at most 400 (the map's values copied?)", be, total)
		}
	}
}
