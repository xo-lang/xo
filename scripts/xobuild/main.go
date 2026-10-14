// Command xobuild makes bin/xo of this checkout the self hosted xo
// (selfhost/xo) of its current sources and prints its path (decision
// 0215: the Go compiler is gone, so an existing xo builds the next one):
//
//	go run ./scripts/xobuild           # build bin/xo if its sources changed
//	XO_SEED=/path/to/xo go run ./scripts/xobuild
//
// The seed and the cache are internal/xotool's: $XO_SEED, the checkout's
// earlier bin/xo, the newest earlier build, `xo` on PATH, the newest
// prebuilt seed (decision 0216: s3://$XO_SEED_BUCKET or $XO_SEED_URL,
// with the aws CLI), else the xo1 of the go-compiler-final tag (built
// once with Go) through scripts/seed-chain.txt. bin/xo finds std at
// <repo>/std. See knowledgebase/infrastructure/toolchain.md
// "Bootstrapping without Go".
package main

import (
	"fmt"
	"os"

	"github.com/xo-lang/xo/internal/xotool"
)

func main() {
	root, err := xotool.Root()
	if err != nil {
		fmt.Fprintln(os.Stderr, "xobuild:", err)
		os.Exit(1)
	}
	bin, err := xotool.Ensure(root, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "xobuild:", err)
		os.Exit(1)
	}
	fmt.Println(bin)
}
