package build

import (
	"strings"
	"testing"
)

// TestClangErrorSanitizerHint: a link failure on a missing compiler-rt
// archive (Ubuntu's clang without libclang-rt-N-dev, seen on EC2 on
// 2026-10-09) names the package to install; other failures do not.
func TestClangErrorSanitizerHint(t *testing.T) {
	tests := []struct {
		name   string
		output string
		hint   bool
	}{
		{"tsan amd64", "/usr/bin/ld: cannot find /usr/lib/llvm-18/lib/clang/18/lib/linux/libclang_rt.tsan-x86_64.a: No such file or directory\nclang: error: linker command failed with exit code 1 (use -v to see invocation)\n", true},
		{"asan arm64", "/usr/bin/ld: cannot find /usr/lib/llvm-18/lib/clang/18/lib/linux/libclang_rt.asan_static-aarch64.a: No such file or directory\n", true},
		{"lld spelling", "ld.lld: error: cannot open /usr/lib/llvm-18/lib/clang/18/lib/linux/libclang_rt.ubsan_standalone-x86_64.a: No such file or directory\n", true},
		{"undefined symbol", "ld: error: undefined symbol: xo_main\n>>> referenced by libclang_rt.profile.a\n", false},
		{"bad IR", "prog.ll:3:1: error: expected top-level entity\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := (&ClangError{Output: tt.output}).Error()
			if !strings.HasPrefix(msg, "clang failed on the generated LLVM IR:\n"+tt.output) {
				t.Fatalf("message lost the clang output: %q", msg)
			}
			if got := strings.Contains(msg, "libclang-rt-"); got != tt.hint {
				t.Errorf("hint = %v, want %v: %q", got, tt.hint, msg)
			}
		})
	}
}
