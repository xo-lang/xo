// Command filesync makes a destination directory mirror a source directory
// (one way).
//
// Usage:
//
//	filesync [--dry-run] [--delete] <src> <dst>
package main

import (
	"errors"
	"flag"
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes the command and returns the process exit status:
// 0 on success, 1 if anything failed, 2 on a usage error.
func run(args []string, stdout, stderr io.Writer) int {
	errp := &printer{w: stderr}
	fset := flag.NewFlagSet("filesync", flag.ContinueOnError)
	fset.SetOutput(stderr)
	dryRun := fset.Bool("dry-run", false, "perform no writes, print what would happen")
	del := fset.Bool("delete", false, "remove files and empty directories in dst that are not in src")
	fset.Usage = func() {
		errp.printf("usage: filesync [--dry-run] [--delete] <src> <dst>\n")
		fset.PrintDefaults()
	}
	if err := fset.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fset.NArg() != 2 {
		fset.Usage()
		return 2
	}

	opts := DefaultOptions(fset.Arg(0), fset.Arg(1))
	opts.DryRun = *dryRun
	opts.Delete = *del
	opts.Out = stdout

	sum, err := Sync(opts)
	if err != nil {
		errp.printf("filesync: %v\n", err)
		return 1
	}
	out := &printer{w: stdout}
	out.printf("summary: copied=%d deleted=%d skipped=%d failed=%d\n",
		sum.Copied, sum.Deleted, sum.Skipped, sum.Failed)
	if out.err != nil || sum.Failed > 0 {
		return 1
	}
	return 0
}
