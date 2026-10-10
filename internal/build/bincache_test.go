package build

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/xo-lang/xo/internal/gen"
)

// TestMain turns the binary cache off for this package's tests: they test
// the builds themselves (reproducible binaries, the Go timers fallback),
// which a cached binary would skip. TestBinCache turns it on for itself.
func TestMain(m *testing.M) {
	os.Setenv("XO_BIN_CACHE", "off")
	os.Exit(m.Run())
}

func binStats() (int, int) {
	BinCacheStats.Lock()
	defer BinCacheStats.Unlock()
	return BinCacheStats.Hits, BinCacheStats.Misses
}

// TestBinCache builds one program twice from two empty build directories
// (as two test processes do): the second build is a hit and runs alike; a
// changed source misses. The native backends are cached the same way.
func TestBinCache(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	t.Setenv("XO_BIN_CACHE", t.TempDir())
	src := filepath.Join(t.TempDir(), "hello")
	os.MkdirAll(src, 0o755)
	write := func(msg string) {
		os.WriteFile(filepath.Join(src, "main.xo"), []byte("fn main(os: Os) {\n  os.stdio.println(\""+msg+"\")\n}\n"), 0o644)
	}
	backends := []string{BackendGo}
	if _, err := Clang(); err == nil {
		backends = append(backends, NativeBackend())
	}
	for _, be := range backends {
		t.Run(be, func(t *testing.T) {
			write("one " + be)
			buildRun := func(want string, hit bool) []byte {
				t.Helper()
				t.Setenv("XO_BUILD_DIR", t.TempDir())
				h0, m0 := binStats()
				res, err := Build([]string{src}, Options{Mode: gen.Build, Backend: be, NoGC: be != BackendGo})
				if err != nil || res.Failed {
					t.Fatalf("build: %v %+v", err, res)
				}
				h1, m1 := binStats()
				if gotHit := h1 == h0+1 && m1 == m0; gotHit != hit {
					t.Errorf("hit %v, want %v (hits %d->%d, misses %d->%d)", gotHit, hit, h0, h1, m0, m1)
				}
				bin := res.Binaries[Host()]
				out, err := exec.Command(bin).Output()
				if err != nil || string(out) != want+"\n" {
					t.Fatalf("run: %v %q, want %q", err, out, want)
				}
				data, err := os.ReadFile(bin)
				if err != nil {
					t.Fatal(err)
				}
				return data
			}
			first := buildRun("one "+be, false)
			if again := buildRun("one "+be, true); !bytes.Equal(first, again) {
				t.Error("the cached binary differs from the built one")
			}
			write("two " + be)
			buildRun("two "+be, false)
			buildRun("two "+be, true)
		})
	}
}
