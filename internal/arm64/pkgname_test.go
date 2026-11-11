package arm64

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestProgramModuleNamedLikeStd: a program package named like a std module
// the program does not import (the golden pkg_named_std's sse, beside
// std/sse) is named by its path in symbols. std's object holds every std
// module whether a program imports it or not, so a bare
// xo_f_sse_2eParser_2efeed met std's at the link when the link pulled
// std's part holding it ("duplicate symbol", the landing_memo golden,
// 2026-11-11); whether it does depends on how std's object is split, so
// the symbols are checked here rather than the link alone
// (gotchas/program-module-named-like-std.md).
func TestProgramModuleNamedLikeStd(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("..", "llvm", "testdata", "golden", "pkg_named_std")
	s := asm(t, dir, runtime.GOOS)
	if strings.Contains(s, "xo_f_sse_2e") {
		t.Errorf("the program's package sse has std/sse's symbol names (xo_f_sse_2e...)")
	}
	if !strings.Contains(s, "pkg__named__std_2fsse_2eParser_2efeed") {
		t.Errorf("no symbol of the program's sse.Parser.feed named by its path")
	}
}
