package repotest

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
