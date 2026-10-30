package arm64

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestMachoBinCache: a link by compiler/macho (no cc) goes through the
// binary cache (macho_bin_key, compiler/xo cache.xo): the first build
// stores the binary, a second identical build takes it from the cache
// (no entry is added), and both are byte for byte the binary a fresh
// link makes with the cache off.
func TestMachoBinCache(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("compiler/macho links for the host on darwin/arm64")
	}
	root := t.TempDir()
	src := filepath.Join(root, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, s string) {
		if err := os.WriteFile(filepath.Join(src, name), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("xo.mod", "module example.com/bincache\nedition preview\n")
	write("main.xo", "fn main(os: Os) {\n  os.stdio.println(\"cached \"+\"link\")\n}\n")
	cache := filepath.Join(root, "bincache")
	build := func(name, binCache string) []byte {
		t.Helper()
		out := filepath.Join(root, name, "prog")
		_, err := xotool.Build([]string{src}, xotool.Options{
			Backend: xotool.BackendArm64,
			Out:     out,
			Flags:   []string{"--ld=macho"},
			// XO_PRUNE_DETACH=off: the prune runs in the build, not in a
			// detached child that may write its stamp files after entries
			// counted them (a loaded main check, 2026-10-30: 3 to 4)
			Env: []string{"XO_BIN_CACHE=" + binCache, "XO_BUILD_DIR=" + filepath.Join(root, "build-"+name), "XO_PRUNE_DETACH=off"},
		})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		b, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	entries := func() int {
		n := 0
		filepath.Walk(cache, func(p string, fi os.FileInfo, err error) error {
			// the run cache (the test cache and, since decision 0360 stage 3, the
			// whole-build entries: run/<xx>/KEY.bin and .m) is not the binary
			// cache, nor are the prune's own files (prune-stamp, prune-stored,
			// prune-lock, ...; compiler/prune clean.xo's rule)
			base := filepath.Base(p)
			if err == nil && !fi.IsDir() && !strings.Contains(filepath.ToSlash(p), "/run/") && !strings.HasPrefix(base, "prune-") && !strings.HasSuffix(base, ".lock") {
				n++
			}
			return nil
		})
		return n
	}
	fresh := build("fresh", "off")
	first := build("first", cache)
	n1 := entries()
	if n1 == 0 {
		t.Fatalf("the first build stored nothing in %s", cache)
	}
	second := build("second", cache)
	if n2 := entries(); n2 != n1 {
		t.Errorf("the second build added cache entries (%d to %d): not a hit", n1, n2)
	}
	if !bytes.Equal(first, fresh) {
		t.Errorf("the stored binary differs from a fresh link (%d and %d bytes)", len(first), len(fresh))
	}
	if !bytes.Equal(second, fresh) {
		t.Errorf("the cached binary differs from a fresh link (%d and %d bytes)", len(second), len(fresh))
	}
}
