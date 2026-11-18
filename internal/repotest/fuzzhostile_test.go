package repotest

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// hostileCall is a call of the hostile dictionary (internal/hostile, or
// an oracle's copy of it).
var hostileCall = regexp.MustCompile(`\bhostile\.(Ints|Uints|Lengths|Digits|Hex|UTF8|CutUTF8|Depths|Nest|Pick)\b`)

// oracleDir is the scripts/stdcorpus directory a harness runs its cases
// from.
var oracleDir = regexp.MustCompile(`"scripts", "stdcorpus", "(\w+)"|stdcorpus/(\w+)`)

// usesHostile: the harness src calls the dictionary, or the oracle it
// names does (a scripts/stdcorpus module's files outside its copy in
// hostile/, or scripts/stdcorpus/NAME*.go).
func usesHostile(t *testing.T, src []byte) bool {
	if hostileCall.Match(src) {
		return true
	}
	for _, m := range oracleDir.FindAllSubmatch(src, -1) {
		dir := string(m[1]) + string(m[2])
		// an oracle module's directory, or a generator file of
		// scripts/stdcorpus itself (asn1.go, asn1fields.go)
		files, _ := filepath.Glob(filepath.Join(root, "scripts", "stdcorpus", dir, "*.go"))
		more, _ := filepath.Glob(filepath.Join(root, "scripts", "stdcorpus", dir+"*.go"))
		files = append(files, more...)
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if hostileCall.Match(data) {
				return true
			}
		}
	}
	return false
}

// TestFuzzHarnessUsesHostile fails when an internal/stdtest fuzz harness
// (*fuzz_test.go) never calls the hostile dictionary, so its edge values
// (Int's bounds, top-bit lengths, digit runs past Int, UTF-8 cut inside a
// rune, deep nesting) are not a fixed share of its cases (task #352; about
// 15 std faults on such input were found by reviews after the module was
// fuzzed: html's long numeric reference, websocket's top-bit length, pg's
// unsigned counts). A harness whose cases come from a scripts/stdcorpus
// oracle passes when the oracle calls it. Harnesses not yet converted are listed in
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
		uses := usesHostile(t, src)
		switch {
		case !uses && !todo[name]:
			t.Errorf("internal/stdtest/%s never calls the hostile dictionary (hostile.Ints, Lengths, Digits, CutUTF8, Nest, ...; internal/hostile): put its values in a share of the cases", name)
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

// TestHostileCopiesCurrent: an oracle in a module of its own holds the
// dictionary as a copy, scripts/stdcorpus/NAME/hostile/hostile.go, that is
// internal/hostile/hostile.go byte for byte (task #352).
func TestHostileCopiesCurrent(t *testing.T) {
	want, err := os.ReadFile(filepath.Join(root, "internal", "hostile", "hostile.go"))
	if err != nil {
		t.Fatal(err)
	}
	copies, err := filepath.Glob(filepath.Join(root, "scripts", "*", "*", "hostile", "hostile.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range copies {
		have, err := os.ReadFile(c)
		if err != nil {
			t.Fatal(err)
		}
		if string(have) != string(want) {
			rel, _ := filepath.Rel(root, c)
			t.Errorf("%s is not internal/hostile/hostile.go: cp internal/hostile/hostile.go %s", rel, rel)
		}
	}
}
