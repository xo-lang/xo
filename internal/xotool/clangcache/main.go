// Command clangcache is the clang the LLVM backend's tests give xo
// ($XO_CLANG, xotool.EnableTestClangCache): it runs the real clang
// ($XO_CLANG_CACHED) through the test clang cache (xotool.RunCachedClang),
// so a program whose IR did not change skips clang.
package main

import (
	"os"

	"github.com/xo-lang/xo/internal/xotool"
)

func main() {
	os.Exit(xotool.RunCachedClang(os.Getenv("XO_CLANG_CACHED"), os.Args[1:]))
}
