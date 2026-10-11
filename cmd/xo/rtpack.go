package main

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/xo-lang/xo/internal/build"
)

// runRTPack is the hidden `xo rtpack` (decision 0157):
//
//	xo rtpack [--target=os/arch] [--cc="cmd args"] [--ar="cmd args"] [-o dir]
//	xo rtpack --status
//	xo rtpack --key [--target=os/arch]
//
// The first form compiles the prebuilt runtime of the arm64 backend for
// the target (default the host) into dir/<os>-<arch> (default
// lib/xo/rtpack, the layout of a release archive). --cc and --ar are
// command lines split at spaces: a cross compiler for another target, for
// example "zig cc -target aarch64-linux-gnu.2.31". --status prints the
// pack this xo uses for the host, or why none; --key prints the key a
// pack for the target must have.
func runRTPack(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("rtpack", flag.ContinueOnError)
	fs.SetOutput(stderr)
	target := fs.String("target", build.Host().String(), "target os/arch")
	cc := fs.String("cc", "", "C compiler command (default the host's C compiler)")
	ar := fs.String("ar", "", "archiver command (default $XO_AR, ar, llvm-ar)")
	out := fs.String("o", "lib/xo/rtpack", "pack root")
	status := fs.Bool("status", false, "print the pack this xo uses for the host")
	key := fs.Bool("key", false, "print the key of a pack for the target")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: xo rtpack [--target=os/arch] [--cc=cmd] [--ar=cmd] [-o dir] | --status | --key")
		return 2
	}
	if *status {
		fmt.Fprintln(stdout, build.RTPackStatus())
		return 0
	}
	ts, err := build.ParseTargets(*target)
	if err != nil || len(ts) != 1 {
		fmt.Fprintf(stderr, "xo rtpack: bad --target %q\n", *target)
		return 2
	}
	if *key {
		k, err := build.RTPackKey(ts[0])
		if err != nil {
			fmt.Fprintln(stderr, "xo rtpack:", err)
			return 1
		}
		fmt.Fprintln(stdout, k)
		return 0
	}
	dir, err := build.BuildRTPack(build.RTPackOptions{Target: ts[0], CC: strings.Fields(*cc), AR: strings.Fields(*ar), Out: *out})
	if err != nil {
		fmt.Fprintln(stderr, "xo rtpack:", err)
		return 1
	}
	fmt.Fprintln(stdout, dir)
	return 0
}
