// Command xobuild makes bin/xo of this checkout the self hosted xo
// (compiler/xo) of its current sources and prints its path (decision
// 0215: the Go compiler is gone, so an existing xo builds the next one):
//
//	go run ./scripts/xobuild                 # build bin/xo if its sources changed
//	XO_SEED=/path/to/xo go run ./scripts/xobuild
//	go run ./scripts/xobuild --from-source   # only the go-compiler-final root and the seed chain
//
// The seed and the cache are internal/xotool's: $XO_SEED, the checkout's
// earlier bin/xo, the newest earlier build, `xo` on PATH, the newest
// prebuilt seed (decision 0216: s3://$XO_SEED_BUCKET or $XO_SEED_URL,
// with the aws CLI). When none builds it fails; the go-compiler-final
// root through scripts/seed-chain.txt (25 to 37 minutes, Go and git) is
// opt in: XO_SEED_FALLBACK=chain after the others, or --from-source
// (XO_SEED_FALLBACK=from-source) alone, for the replay from source and a
// new platform (a build of the same key already in the cache is reused:
// point XO_CHECKOUT_CACHE at an empty directory for a full replay).
// bin/xo finds std at <repo>/std. See
// knowledgebase/infrastructure/toolchain.md "Bootstrapping without Go".
package main

import (
	"fmt"
	"os"

	"github.com/xo-lang/xo/internal/xotool"
)

func main() {
	for _, a := range os.Args[1:] {
		switch a {
		case "--from-source", "-from-source":
			os.Setenv("XO_SEED_FALLBACK", "from-source")
		default:
			fmt.Fprintf(os.Stderr, "xobuild: unknown argument %q (usage: xobuild [--from-source])\n", a)
			os.Exit(2)
		}
	}
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
