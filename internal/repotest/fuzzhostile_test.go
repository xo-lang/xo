package repotest

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// hostileCall is a call of internal/stdtest's hostile dictionary
// (hostilevals_test.go).
var hostileCall = regexp.MustCompile(`\b(Hostile(Ints|Uints|Lengths|Digits|Hex|UTF8|Depths|Pick)|CutUTF8|Nest)\(`)

// TestFuzzHarnessUsesHostile fails when an internal/stdtest fuzz harness
// (*fuzz_test.go) never calls the hostile dictionary, so its edge values
// (Int's bounds, top-bit lengths, digit runs past Int, UTF-8 cut inside a
// rune, deep nesting) are not a fixed share of its cases (task #352; about
// 15 std faults on such input were found by reviews after the module was
// fuzzed: html's long numeric reference, websocket's top-bit length, pg's
// unsigned counts). Harnesses not yet converted are listed in
// testdata/fuzz-hostile-todo.txt; a listed harness that calls it fails
// too, so the list only shrinks.
func TestFuzzHarnessUsesHostile(t *testing.T) {
	todo := map[string]bool{}
	data, err := os.ReadFile(filepath.Join(root, "internal", "repotest", "testdata", "fuzz-hostile-todo.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			todo[line] = true
		}
	}
	files, err := filepath.Glob(filepath.Join(root, "internal", "stdtest", "*fuzz_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no internal/stdtest/*fuzz_test.go")
	}
	seen := map[string]bool{}
	for _, f := range files {
		name := filepath.Base(f)
		seen[name] = true
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		uses := hostileCall.Match(src)
		switch {
		case !uses && !todo[name]:
			t.Errorf("internal/stdtest/%s never calls the hostile dictionary (HostileInts, HostileLengths, HostileDigits, CutUTF8, Nest, ...; hostilevals_test.go): put its values in a share of the cases", name)
		case uses && todo[name]:
			t.Errorf("internal/stdtest/%s calls the hostile dictionary: drop it from internal/repotest/testdata/fuzz-hostile-todo.txt", name)
		}
	}
	for name := range todo {
		if !seen[name] {
			t.Errorf("internal/repotest/testdata/fuzz-hostile-todo.txt lists %s, which is gone: drop its line", name)
		}
	}
}
