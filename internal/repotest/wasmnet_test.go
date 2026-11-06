package repotest

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// wasmNetCall matches the calls that reach the network, which no wasm target
// has (WASI preview 1 has no sockets; a wasip2 command imports none): a Net
// capability's listen or dial, an http client, a name lookup, udp.
var wasmNetCall = regexp.MustCompile(`\.listen\(|\.dial\(|\bhttp\.client\(|\bnet\.lookup|\budp\.`)

// TestWasmUnsupportedListsNetGoldens: an LLVM golden whose sources call the
// network is listed in internal/selfhost/testdata/wasm_unsupported.txt, read
// from the sources alone (no xo build, so pushq's fast checks run it).
// TestSelfhostWasm is the real check, but it needs compiler/wasm and Node,
// and net_dial_dual_stack reached main unlisted on 2026-11-02 (main's
// TestSelfhostWasm failed in the next full tier). On 2026-11-03 the pattern
// matched 57 goldens, every one listed for the network, and no other.
func TestWasmUnsupportedListsNetGoldens(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(root, "internal", "selfhost", "testdata", "wasm_unsupported.txt"))
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	for _, l := range strings.Split(string(b), "\n") {
		l, _, _ = strings.Cut(l, "#")
		if l = strings.TrimSpace(l); l != "" {
			listed[l] = true
		}
	}
	dirs, err := filepath.Glob(filepath.Join(root, "internal", "llvm", "testdata", "golden", "*"))
	if err != nil || len(dirs) < 100 {
		t.Fatalf("found %d goldens (%v): the scan read the wrong tree", len(dirs), err)
	}
	matched := 0
	for _, d := range dirs {
		files, _ := filepath.Glob(filepath.Join(d, "*.xo"))
		for _, f := range files {
			// "*.xo" also matches a build's .xo cache directory
			// (use_c_add's, left by a golden run in the tree)
			if fi, err := os.Stat(f); err == nil && fi.IsDir() {
				continue
			}
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if wasmNetCall.Match(src) {
				matched++
				if name := filepath.Base(d); !listed[name] {
					t.Errorf("golden %s calls the network (%s) but is not listed in internal/selfhost/testdata/wasm_unsupported.txt; add `%s # wasip1 # network: WASI preview1 has no sockets`", name, filepath.Base(f), name)
				}
				break
			}
		}
	}
	if matched < 20 {
		t.Fatalf("only %d goldens call the network: the pattern no longer matches how they do", matched)
	}
}
