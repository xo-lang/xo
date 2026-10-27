package arm64

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// chainGoldens are the goldens built into one cache by
// TestChainKeysSharedCache: the TLS and HTTP programs whose output a
// shared cache under XO_CHAIN_KEYS=1 changed (2026-10-27), and two more
// with constants and debug rows.
var chainGoldens = []string{
	"tls_coalesced_finish", "http_serve_content", "http_gzip_responses",
	"http_header_fields", "http_tls_keys", "http_conn_fault", "http_tls",
	"http_cookies", "time_named_layouts",
}

// TestChainKeysSharedCache: goldens built one after another into one
// back-end cache with XO_CHAIN_KEYS=1 (decision 0382 stage 3), cold and
// then warm, are each byte for byte the build with the cache off. A key
// that hashes a body's front end entry in place of the body must also
// cover what a replay sets beyond the entry (ssa key_replayed): the
// first version took another build's constant numbers, and TLS goldens
// read the wrong constants, while XO_BACKEND_CACHE=verify passed.
func TestChainKeysSharedCache(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-chainkeys-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	shared := filepath.Join(work, "cache")
	build := func(g string, env ...string) []byte {
		t.Helper()
		out := filepath.Join(work, "bin-"+g)
		os.Remove(out)
		cmd := testtmp.Guard(exec.Command(xo, "build", "--backend=arm64", "-o", out, "."))
		cmd.Dir = filepath.Join("..", "llvm", "testdata", "golden", g)
		cmd.Env = append(os.Environ(), env...)
		if log, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("xo build %s %v: %v\n%s", g, env, err, log)
		}
		b, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	off := map[string][]byte{}
	for _, g := range chainGoldens {
		off[g] = build(g, "XO_BACKEND_CACHE=off", "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "off"))
	}
	for _, pass := range []string{"cold", "warm"} {
		for _, g := range chainGoldens {
			got := build(g, "XO_CHAIN_KEYS=1", "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+shared)
			if !bytes.Equal(got, off[g]) {
				t.Errorf("%s (%s): the build through a cache shared with the other goldens differs from the cache-off build", g, pass)
			}
		}
	}
	// and verify finds no entry its key took from another body
	for _, g := range chainGoldens {
		build(g, "XO_CHAIN_KEYS=1", "XO_BACKEND_CACHE=verify", "XO_BACKEND_CACHE_DIR="+shared)
	}
}
