package repotest

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestRuntimeIntrinsicsLowered fails when an rt.* intrinsic declared in
// std/native/rt.xo (a top level fn with no body) has no lowering in a
// backend that compiles std/native, and the gap is not in
// testdata/intrinsic_gaps.txt with its reason: rt.mem_copy_block had an
// arm64, amd64, and LLVM lowering but none in the wasm runtime, and 20
// wasm goldens were unsupported until the slow TestSelfhostWasm ran
// (gotchas/new-rt-intrinsic-needs-wasm.md). A backend lowers a name when
// its sources name it ("rt.NAME", "NAME", or the wasm runtime's $NAME);
// arm64 (whose lowering amd64 retargets) and LLVM also through a function
// of std/native/llrt/cabi*.xo, the exports their calls reach, or the
// exports of compiler/ssa/exports.xo. A name compiler/ssa handles itself
// counts for every backend only when listed. A gap that closes fails too,
// so the list stays current.
func TestRuntimeIntrinsicsLowered(t *testing.T) {
	read := func(rel string) string {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	glob := func(pat string) string {
		files, err := filepath.Glob(filepath.Join(root, pat))
		if err != nil || len(files) == 0 {
			t.Fatalf("no files for %s: %v", pat, err)
		}
		var b strings.Builder
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			b.Write(data)
			b.WriteByte('\n')
		}
		return b.String()
	}
	decl := regexp.MustCompile(`(?m)^fn (\w+)(\[[^\]]*\])?\([^\n]*$`)
	var names []string
	seen := map[string]bool{}
	for _, m := range decl.FindAllStringSubmatch(read("std/native/rt.xo"), -1) {
		if strings.HasSuffix(strings.TrimSpace(m[0]), "{") || seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		names = append(names, m[1])
	}
	sort.Strings(names)
	if len(names) < 100 {
		t.Fatalf("only %d intrinsics in std/native/rt.xo: the declaration pattern no longer matches", len(names))
	}
	cabi := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^(?:pub )?fn (\w+)\(`).FindAllStringSubmatch(glob("std/native/llrt/cabi*.xo"), -1) {
		cabi[m[1]] = true
	}
	exports := read("compiler/ssa/exports.xo")
	sources := map[string]string{
		"arm64": glob("compiler/arm64/*.xo") + exports,
		"llvm":  glob("compiler/llvm/*.xo") + exports,
		"wasm":  glob("compiler/wasm/*.xo"),
	}
	lowered := func(be, n string) bool {
		s := sources[be]
		if strings.Contains(s, `"rt.`+n+`"`) || strings.Contains(s, `"`+n+`"`) {
			return true
		}
		if be == "wasm" {
			for _, c := range []string{" ", ")", "\n"} {
				if strings.Contains(s, "$"+n+c) {
					return true
				}
			}
			return false
		}
		return cabi[n]
	}
	// the allowed gaps: backend name reason
	gaps := map[string]string{}
	f, err := os.Open(filepath.Join(root, "internal/repotest/testdata/intrinsic_gaps.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		p := strings.SplitN(line, " ", 3)
		if len(p) < 3 || strings.TrimSpace(p[2]) == "" {
			t.Errorf("intrinsic_gaps.txt: %q: want `backend name reason`", line)
			continue
		}
		gaps[p[0]+" "+p[1]] = p[2]
	}
	for _, be := range []string{"arm64", "llvm", "wasm"} {
		for _, n := range names {
			key := be + " " + n
			_, allowed := gaps[key]
			has := lowered(be, n)
			switch {
			case !has && !allowed:
				t.Errorf("rt.%s (std/native/rt.xo) has no lowering in the %s backend: add one (for wasm, compiler/wasm rt*.xo and its direct list), or a line `%s %s <reason>` to internal/repotest/testdata/intrinsic_gaps.txt", n, be, be, n)
			case has && allowed:
				t.Errorf("intrinsic_gaps.txt lists `%s` but the %s backend lowers rt.%s now: remove the line", key, be, n)
			}
			delete(gaps, key)
		}
	}
	for k := range gaps {
		t.Errorf("intrinsic_gaps.txt lists `%s`, which std/native/rt.xo does not declare: remove the line", k)
	}
}
