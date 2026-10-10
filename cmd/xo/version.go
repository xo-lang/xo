package main

import (
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
)

// version is the toolchain version (decision 0089). Xo stays on 0.x minor
// versions until the owner declares 1.0; raise the minor at each milestone
// and add the matching CHANGELOG.md entry and git tag.
const version = "0.9.0"

func runVersion(args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, "usage: xo version")
		return 2
	}
	rev, dirty := "", false
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				rev = s.Value
			case "vcs.modified":
				dirty = s.Value == "true"
			}
		}
	}
	line := "xo " + version
	if rev != "" {
		if len(rev) > 12 {
			rev = rev[:12]
		}
		line += " (" + rev
		if dirty {
			line += ", modified"
		}
		line += ")"
	}
	fmt.Fprintf(stdout, "%s %s/%s\n", line, runtime.GOOS, runtime.GOARCH)
	return 0
}
