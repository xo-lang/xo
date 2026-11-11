package repotest

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestStdRuntimeCallsHaveGoLowering fails when a std module the Go backend
// compiles from its native implementation (std/go_compiled.txt, decision
// 0312: std/native/NAME.xo and its tests) calls an rt.*
// intrinsic that std/rt (the Go backend's runtime) does not define as
// func Rt_NAME, and the call is not in testdata/go_rt_gaps.txt with its
// reason. The Go backend turns rt.NAME into rt.Rt_NAME, so the gap
// otherwise shows only when a Go backend build of the module fails
// (2026-11-07: netip's string4 with rt.rs_push_words failed `undefined
// rt.Rt_rs_push_words`, and the speedup became a prelude API instead,
// decision 0561). The wasm counterpart is TestRuntimeIntrinsicsLowered. A
// listed gap that closes fails too, so the list stays current.
func TestStdRuntimeCallsHaveGoLowering(t *testing.T) {
	list, err := os.ReadFile(filepath.Join(root, "std/go_compiled.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, l := range strings.Split(string(list), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		// a merged module (decision 0588) has no native file left; a
		// name that is no std module at all means the list's format
		// changed (the check counted 20 files until merges took the list
		// below that, 2026-11-11)
		if _, err := os.Stat(filepath.Join(root, "std", l+".xo")); err != nil {
			t.Fatalf("std/go_compiled.txt: %q is no std module (std/%s.xo): the list's format changed?", l, l)
		}
		for _, f := range []string{"std/native/" + l + ".xo", "std/native/" + l + "_test.xo"} {
			if _, err := os.Stat(filepath.Join(root, f)); err == nil {
				files = append(files, filepath.Join(root, f))
			}
		}
	}
	if len(files) == 0 {
		t.Fatal("std/go_compiled.txt: no implementation files: the list's format changed?")
	}
	call := regexp.MustCompile(`\brt\.([a-z_][a-z_0-9]*)\s*[\[(]`)
	callers := map[string][]string{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if c := strings.Index(line, "//"); c >= 0 {
				line = line[:c]
			}
			for _, m := range call.FindAllStringSubmatch(line, -1) {
				rel, _ := filepath.Rel(root, f)
				callers[m[1]] = append(callers[m[1]], rel+":"+strconv.Itoa(i+1))
			}
		}
	}
	goFiles, err := filepath.Glob(filepath.Join(root, "std/rt/*.go"))
	if err != nil || len(goFiles) == 0 {
		t.Fatalf("std/rt/*.go: %v", err)
	}
	def := regexp.MustCompile(`(?m)^func Rt_([a-z_][a-z_0-9]*)[\[(]`)
	defined := map[string]bool{}
	for _, f := range goFiles {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range def.FindAllStringSubmatch(string(data), -1) {
			defined[m[1]] = true
		}
	}
	gaps := map[string]string{}
	data, err := os.ReadFile(filepath.Join(root, "internal/repotest/testdata/go_rt_gaps.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, reason, _ := strings.Cut(line, " ")
		if strings.TrimSpace(reason) == "" {
			t.Errorf("go_rt_gaps.txt: %s has no reason", name)
		}
		gaps[name] = reason
	}
	var names []string
	for n := range callers {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if defined[n] {
			if _, ok := gaps[n]; ok {
				t.Errorf("rt.%s: std/rt defines Rt_%s now; remove it from testdata/go_rt_gaps.txt", n, n)
			}
			continue
		}
		if _, ok := gaps[n]; !ok {
			t.Errorf("rt.%s (%s): the Go backend has no rt.Rt_%s; add it to std/rt (as Rt_cast in std/rt/str.go), or the call to testdata/go_rt_gaps.txt with why the module never builds on the Go backend", n, strings.Join(callers[n], ", "), n)
		}
	}
	for n := range gaps {
		if _, ok := callers[n]; !ok {
			t.Errorf("go_rt_gaps.txt: no std module calls rt.%s any more; remove its line", n)
		}
	}
}
