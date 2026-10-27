package arm64

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
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

// TestChainKeysRebuildReuses: under XO_CHAIN_KEYS=1 an unchanged rebuild
// reuses every function of the build before it. A function's body id
// (Module.body_ids, which the reference counting's keys hash) is the
// hash of its form after the cleanup, not the cleanup's key: a cold
// linux/arm64 build after a host build runs its front end twice (the
// module check cache checks the program again), the first pass keyed
// its bodies without front end entries, the second by them, and the
// rebuild, one pass, took the first pass's ids, so every key chained
// from them missed (functions 1 of 723 reused, 2026-10-27).
func TestChainKeysRebuildReuses(t *testing.T) {
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
	work, err := testtmp.Dir("xo-chainre-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	src := filepath.Join(work, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for i := 0; i < 300; i++ {
		if i == 299 {
			fmt.Fprintf(&b, "fn f%d(x: Int) -> Int {\n  x * 3\n}\n\n", i)
			continue
		}
		fmt.Fprintf(&b, "fn f%d(x: Int) -> Int {\n  if x > %d {\n    f%d(x - 1) + %d\n  } else {\n    x * 3\n  }\n}\n\n", i, i, i+1, i)
	}
	b.WriteString("fn main(os: Os) {\n  os.stdio.println(\"${f0(400)}\")\n}\n")
	if err := os.WriteFile(filepath.Join(src, "main.xo"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	build := func(target string) string {
		t.Helper()
		args := []string{"build", "--backend=arm64", "--phase-times", "-o", filepath.Join(work, "prog-"+strings.ReplaceAll(target, "/", "-"))}
		if target != "" {
			args = append(args, "--targets="+target)
		}
		cmd := testtmp.Guard(exec.Command(xo, append(args, ".")...))
		cmd.Dir = src
		cmd.Env = append(os.Environ(), "XO_CHAIN_KEYS=1", "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "cache"))
		log, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("xo build %s: %v\n%s", target, err, log)
		}
		return string(log)
	}
	fns := regexp.MustCompile(`cache functions (\d+)/(\d+) reused`)
	build("")
	build("linux/arm64")
	for _, target := range []string{"", "linux/arm64"} {
		log := build(target)
		m := fns.FindStringSubmatch(log)
		if m == nil {
			t.Fatalf("%q: no cache functions line:\n%s", target, log)
		}
		if m[1] != m[2] {
			t.Errorf("%q: an unchanged rebuild reused functions %s of %s, want all:\n%s", target, m[1], m[2], log)
		}
	}
}
