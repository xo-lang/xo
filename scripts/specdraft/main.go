// Command specdraft moves the spec's draft number on at a minor release
// (decision 0089): given the released version 0.N.P it makes spec/core.md
// and spec/std.md "Status: draft 0.N+1" and gives core.md an empty
// "## Changes in 0.N+1" section above the last one. The release scripts
// (relcommit.sh, relmain.sh) run it; it changes nothing when the draft is
// already 0.N+1, so a patch release or a second run is a no-op.
// internal/repotest TestSpecDraftFollowsVersion checks the result.
//
//	go run ./scripts/specdraft [-root DIR] [-date YYYY-MM-DD] 0.N.P
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func main() {
	root := flag.String("root", ".", "repository root")
	date := flag.String("date", time.Now().Format("2006-01-02"), "the release date")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: specdraft [-root DIR] [-date YYYY-MM-DD] 0.N.P")
		os.Exit(2)
	}
	m := regexp.MustCompile(`^0\.(\d+)\.\d+$`).FindStringSubmatch(flag.Arg(0))
	if m == nil {
		fmt.Fprintf(os.Stderr, "specdraft: %q is not a version 0.N.P\n", flag.Arg(0))
		os.Exit(2)
	}
	rel, _ := strconv.Atoi(m[1])
	next := rel + 1
	status := regexp.MustCompile(`(?m)^Status: draft 0\.(\d+)\b.*$`)
	for _, f := range []string{"core.md", "std.md"} {
		p := filepath.Join(*root, "spec", f)
		b, err := os.ReadFile(p)
		if err != nil {
			fmt.Fprintln(os.Stderr, "specdraft:", err)
			os.Exit(1)
		}
		s := string(b)
		sm := status.FindStringSubmatchIndex(s)
		if sm == nil {
			fmt.Fprintf(os.Stderr, "specdraft: spec/%s has no \"Status: draft 0.N\" line\n", f)
			os.Exit(1)
		}
		if n, _ := strconv.Atoi(s[sm[2]:sm[3]]); n >= next {
			fmt.Printf("spec/%s: draft 0.%d already\n", f, n)
			continue
		}
		line := fmt.Sprintf("Status: draft 0.%d (%s; 0.%d released as toolchain v%s, decision 0089)", next, *date, rel, flag.Arg(0))
		if f == "std.md" {
			line = fmt.Sprintf("Status: draft 0.%d (%s; the same draft number as `spec/core.md`, decision 0089)", next, *date)
		}
		s = s[:sm[0]] + line + s[sm[1]:]
		if f == "core.md" {
			head := fmt.Sprintf("## Changes in 0.%d\n", next)
			i := strings.Index(s, "\n## Changes in ")
			if i < 0 {
				fmt.Fprintln(os.Stderr, "specdraft: spec/core.md has no \"## Changes in\" section")
				os.Exit(1)
			}
			if !strings.Contains(s, "\n"+head) {
				s = s[:i+1] + head + "\nNone yet.\n\n" + s[i+1:]
			}
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "specdraft:", err)
			os.Exit(1)
		}
		fmt.Printf("spec/%s: draft 0.%d\n", f, next)
	}
}
