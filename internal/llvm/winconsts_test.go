package llvm

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestWinConsts compiles std/llrt/testdata/win_consts.c, the static
// asserts of the Windows constants and layouts the native runtime's Xo
// hard codes (sock.xo, cabi_net.xo, poll.xo; decision 0234), for
// windows/amd64 and windows/arm64 with the Windows cross toolchain:
// XO_CLANG and XO_WINSYSROOT, else the Mac's set-up in the user cache
// (knowledgebase/infrastructure/toolchain.md "Windows cross builds on the
// Mac"). It skips where neither is there.
func TestWinConsts(t *testing.T) {
	clang, sysroot := os.Getenv("XO_CLANG"), os.Getenv("XO_WINSYSROOT")
	if sysroot == "" {
		if cache, err := os.UserCacheDir(); err == nil {
			x := filepath.Join(cache, "xo", "winxc")
			sysroot = filepath.Join(x, "winsysroot")
			if clang == "" {
				clang = filepath.Join(x, "LLVM-23.1.3-macOS-ARM64", "bin", "clang")
			}
		}
	}
	if clang == "" {
		clang = "clang"
	}
	if _, err := os.Stat(sysroot); err != nil {
		t.Skip("no Windows sysroot (XO_WINSYSROOT): the Windows cross toolchain is not set up")
	}
	if _, err := exec.LookPath(clang); err != nil {
		t.Skipf("no clang for the Windows cross build: %v", err)
	}
	src := filepath.Join(rtDir, "testdata", "win_consts.c")
	for _, triple := range []string{"x86_64-pc-windows-msvc", "aarch64-pc-windows-msvc"} {
		out, err := exec.Command(clang, "--target="+triple, "-Xmicrosoft-windows-sys-root", sysroot, "-fsyntax-only", src).CombinedOutput()
		if err != nil {
			t.Errorf("%s: %v\n%s", triple, err, out)
		}
	}
}
