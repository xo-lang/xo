package repotest

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestPEImportsCoverSysWindows: every Windows API function the native
// runtime declares (std/native/sys_windows.xo) has a DLL in the PE link's
// import table (compiler/macho/peimports.xo), else a windows/amd64 link of
// a program that calls it fails with "undefined symbol" (std/udp's
// sendto and recvfrom, and FlushFileBuffers, 2026-10-29).
func TestPEImportsCoverSysWindows(t *testing.T) {
	decl, err := os.ReadFile(filepath.Join(root, "std", "native", "sys_windows.xo"))
	if err != nil {
		t.Fatal(err)
	}
	imp, err := os.ReadFile(filepath.Join(root, "compiler", "macho", "peimports.xo"))
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, m := range regexp.MustCompile(`"(\w+)":\s*"[^"]+\.dll"`).FindAllSubmatch(imp, -1) {
		have[string(m[1])] = true
	}
	fns := regexp.MustCompile(`(?m)^fn (\w+)\(`).FindAllSubmatch(decl, -1)
	if len(fns) == 0 {
		t.Fatal("no functions found in sys_windows.xo")
	}
	for _, m := range fns {
		if !have[string(m[1])] {
			t.Errorf("sys_windows.xo declares %s, which peimports.xo does not import", m[1])
		}
	}
}

// TestPEImportsMatchInputs: the PE import table holds exactly the names
// scripts/win-import-table.py generates it from, sys_windows.xo's
// functions and scripts/win-imports-extra.txt's, in the generator's
// order, so regenerating it with an SDK changes no entry (an entry added
// to the table by hand was dropped by a regeneration, 2026-11-07, and an
// SDK's IPHLPAPI.DLL spelling hid GetAdaptersAddresses from the test
// above; the generator writes .dll).
func TestPEImportsMatchInputs(t *testing.T) {
	decl, err := os.ReadFile(filepath.Join(root, "std", "native", "sys_windows.xo"))
	if err != nil {
		t.Fatal(err)
	}
	extra, err := os.ReadFile(filepath.Join(root, "scripts", "win-imports-extra.txt"))
	if err != nil {
		t.Fatal(err)
	}
	imp, err := os.ReadFile(filepath.Join(root, "compiler", "macho", "peimports.xo"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^fn ([A-Za-z_][A-Za-z0-9_]*)\(`).FindAllSubmatch(decl, -1) {
		want[string(m[1])] = true
	}
	for _, line := range strings.Split(string(extra), "\n") {
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		if line = strings.TrimSpace(line); line != "" {
			want[line] = true
		}
	}
	var got []string
	for _, m := range regexp.MustCompile(`(?m)^    "(\w+)": "([^"]+)",$`).FindAllSubmatch(imp, -1) {
		name, dll := string(m[1]), string(m[2])
		got = append(got, name)
		if !want[name] {
			t.Errorf("peimports.xo imports %s, which neither sys_windows.xo nor scripts/win-imports-extra.txt names: a regeneration drops it; add it to win-imports-extra.txt", name)
		}
		if !strings.HasSuffix(dll, ".dll") {
			t.Errorf("peimports.xo gives %s the DLL %q: the generator writes the extension .dll", name, dll)
		}
		delete(want, name)
	}
	for name := range want {
		t.Errorf("%s is an input of scripts/win-import-table.py but not in peimports.xo: regenerate the table", name)
	}
	if !sort.StringsAreSorted(got) {
		t.Errorf("peimports.xo's entries are not in the generator's (sorted) order: regenerate the table")
	}
}

// TestPEImportsInSeed: the PE import table names no function the seed's
// release table lacks (scripts/win-imports-seed.txt, that release's
// names; the seed is the newest release, decision 0216). A function
// std/native declares in sys_windows.xo is imported by name through the
// table compiled into the linking xo, so an xo older than the entry, the
// seed among them, fails to link xo itself on Windows ("undefined symbol
// GetThreadId", 2026-11-22, task #689: 0f5b7b96c0 added GetThreadId and
// NtGetNextThread after 0.55.0). Find such a function at run time instead
// (std/native/llrt/process.xo win_proc), or add it once the release
// before it is the seed. The list is a file, not the newest tag's table:
// an offload gate's clone holds no newer tags than its first fetch (its
// bundles carry branches only; v0.37.0 there on 2026-11-22). Where the
// tags are present the list is checked against its release's table, and
// a newer release tag asks for the list to be regenerated.
func TestPEImportsInSeed(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(root, "scripts", "win-imports-seed.txt"))
	if err != nil {
		t.Fatal(err)
	}
	rel, have := "", map[string]bool{}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "release "); ok {
			rel = v
		} else if line != "" && !strings.HasPrefix(line, "#") {
			have[line] = true
		}
	}
	if rel == "" || len(have) == 0 {
		t.Fatalf("scripts/win-imports-seed.txt: want a \"release vX.Y.Z\" line and its import names, got release %q and %d names", rel, len(have))
	}
	imp, err := os.ReadFile(filepath.Join(root, "compiler", "macho", "peimports.xo"))
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`(?m)^    "(\w+)": "[^"]+",$`)
	for _, m := range re.FindAllSubmatch(imp, -1) {
		if !have[string(m[1])] {
			t.Errorf("peimports.xo imports %s, which %s's does not: the %s seed cannot link a windows program that calls it; find it at run time (process.win_proc)", m[1], rel, rel)
		}
	}
	// with the tags at hand: the list is its release's, and no newer
	// release has become the seed without it
	if old, err := exec.Command("git", "-C", root, "show", rel+":compiler/macho/peimports.xo").Output(); err == nil {
		tagged := map[string]bool{}
		for _, m := range re.FindAllSubmatch(old, -1) {
			tagged[string(m[1])] = true
		}
		for n := range have {
			if !tagged[n] {
				t.Errorf("scripts/win-imports-seed.txt names %s, which %s's peimports.xo does not: regenerate the list", n, rel)
			}
		}
		for n := range tagged {
			if !have[n] {
				t.Errorf("%s's peimports.xo imports %s, which scripts/win-imports-seed.txt lacks: regenerate the list", rel, n)
			}
		}
	}
	if tag, err := exec.Command("git", "-C", root, "describe", "--tags", "--abbrev=0", "--match", "v[0-9]*", "HEAD").Output(); err == nil {
		if newest := strings.TrimSpace(string(tag)); newest != rel && versionLess(rel, newest) {
			t.Errorf("the newest release before HEAD is %s, but scripts/win-imports-seed.txt is %s's: regenerate it from %s (its first comment lines say how)", newest, rel, newest)
		}
	}
}

// versionLess: vA.B.C before vX.Y.Z.
func versionLess(a, b string) bool {
	pa, pb := strings.Split(strings.TrimPrefix(a, "v"), "."), strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		x, _ := strconv.Atoi(pa[i])
		y, _ := strconv.Atoi(pb[i])
		if x != y {
			return x < y
		}
	}
	return len(pa) < len(pb)
}
