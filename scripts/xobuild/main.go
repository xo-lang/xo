// Command xobuild makes bin/xo of this checkout the self hosted xo
// (compiler/xo) of its current sources and prints its path (decision
// 0215: the Go compiler is gone, so an existing xo builds the next one):
//
//	go run ./scripts/xobuild                 # build bin/xo if its sources changed
//	XO_SEED=/path/to/xo go run ./scripts/xobuild
//	go run ./scripts/xobuild --from-source   # only the go-compiler-final root and the seed chain
//	go run ./scripts/xobuild --key           # print the source key, build nothing
//	go run ./scripts/xobuild --stamp-args=XO # the --stamp flags of a build of compiler/xo by XO, one per line
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
	"strings"

	"github.com/xo-lang/xo/internal/xotool"
)

func main() {
	keyOnly := false
	stampBy := ""
	for _, a := range os.Args[1:] {
		switch {
		case a == "--from-source" || a == "-from-source":
			os.Setenv("XO_SEED_FALLBACK", "from-source")
		case a == "--key" || a == "-key":
			keyOnly = true
		case strings.HasPrefix(a, "--stamp-args=") && len(a) > len("--stamp-args="):
			stampBy = strings.TrimPrefix(a, "--stamp-args=")
		default:
			fmt.Fprintf(os.Stderr, "xobuild: unknown argument %q (usage: xobuild [--from-source] [--key] [--stamp-args=XO])\n", a)
			os.Exit(2)
		}
	}
	root, err := xotool.Root()
	if err != nil {
		fmt.Fprintln(os.Stderr, "xobuild:", err)
		os.Exit(1)
	}
	if stampBy != "" {
		// the provenance flags (decision 0264) of a build of this
		// checkout's compiler/xo by stampBy, for builds xobuild does not
		// make (scripts/release.sh: each target's xo reports the commit
		// it was built from)
		args := xotool.StampArgs(root, stampBy)
		if args == nil {
			fmt.Fprintln(os.Stderr, "xobuild: no stamp for this checkout (no compiler/xo provenance.xo, or the source key failed)")
			os.Exit(1)
		}
		for _, a := range args {
			fmt.Println(a)
		}
		return
	}
	if keyOnly {
		// the source key a build of this checkout has (decision 0264:
		// the bench scripts compare it with `xo version --json`)
		key, err := xotool.Key(root)
		if err != nil {
			fmt.Fprintln(os.Stderr, "xobuild:", err)
			os.Exit(1)
		}
		fmt.Println(key)
		return
	}
	// which tree this builds for, on stderr: an agent's shell that resets
	// its directory between commands ran xobuild in another worktree, and
	// the path it printed was that tree's xo (2026-10-21)
	if key, err := xotool.Key(root); err == nil {
		fmt.Fprintf(os.Stderr, "xobuild: %s (key %s)\n", root, key)
	}
	bin, err := xotool.Ensure(root, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "xobuild:", err)
		os.Exit(1)
	}
	fmt.Println(bin)
}
