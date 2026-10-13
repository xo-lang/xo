package build

import (
	"reflect"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/diag"
)

// TestTargetUnsupported checks XO1504 (decision 0197): since stage 2
// (processes, 2026-10-12) every runtime part is ported to Windows, so no
// define stops a build; a part added to unsupportedOnWindows does.
func TestTargetUnsupported(t *testing.T) {
	for _, tc := range []struct {
		goos  string
		flags []string
		want  string // "" for no diagnostic, else a substring of its message
	}{
		{"windows", nil, ""},
		{"windows", []string{"-DXO_FS", "-DXO_STDIN", "-DXO_TIME"}, ""},
		{"windows", []string{"-DXO_FS", "-DXO_PROC"}, ""},
		{"linux", []string{"-DXO_PROC"}, ""},
		{"darwin", []string{"-DXO_PROC"}, ""},
	} {
		ds := TargetUnsupported(tc.goos, tc.flags)
		if tc.want == "" {
			if len(ds) != 0 {
				t.Errorf("%s %v: got %v, want none", tc.goos, tc.flags, ds)
			}
			continue
		}
		if len(ds) != 1 || ds[0].Code != string(diag.TargetUnsupported) || !strings.Contains(ds[0].Message, tc.want) {
			t.Errorf("%s %v: got %v, want one XO1504 naming %q", tc.goos, tc.flags, ds, tc.want)
		}
	}
}

// TestLLVMCrossTriple checks the cross builds of the LLVM backend
// (decision 0197): between the two Windows architectures only.
func TestLLVMCrossTriple(t *testing.T) {
	for _, tc := range []struct {
		host, t Target
		want    string
	}{
		{Target{"windows", "amd64"}, Target{"windows", "arm64"}, "aarch64-pc-windows-msvc"},
		{Target{"windows", "arm64"}, Target{"windows", "amd64"}, "x86_64-pc-windows-msvc"},
		{Target{"windows", "amd64"}, Target{"windows", "amd64"}, ""},
		{Target{"windows", "amd64"}, Target{"linux", "arm64"}, ""},
		{Target{"darwin", "arm64"}, Target{"darwin", "amd64"}, ""},
		{Target{"linux", "amd64"}, Target{"windows", "amd64"}, ""},
	} {
		got, ok := LLVMCrossTriple(tc.host, tc.t)
		if got != tc.want || ok != (tc.want != "") {
			t.Errorf("%v on %v: got %q %v, want %q", tc.t, tc.host, got, ok, tc.want)
		}
	}
}

// TestSysLinkArgs checks the system part of a native link: lld on
// Windows (LTO bitcode; the runtime names its import libraries), libm and
// pthreads elsewhere.
func TestSysLinkArgs(t *testing.T) {
	if got := sysLinkArgs("windows"); !reflect.DeepEqual(got, []string{"-fuse-ld=lld"}) {
		t.Errorf("windows: %v", got)
	}
	if got := sysLinkArgs("linux"); !reflect.DeepEqual(got, []string{"-lm", "-lpthread"}) {
		t.Errorf("linux: %v", got)
	}
}
