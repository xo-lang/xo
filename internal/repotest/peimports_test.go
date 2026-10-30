package repotest

import (
	"os"
	"path/filepath"
	"regexp"
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
