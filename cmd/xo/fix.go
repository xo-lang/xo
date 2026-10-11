package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/fix"
)

// runFix implements `xo fix [--dry-run] [--json] [--from=draft-0.N]
// [--edition=preview] [dirs...]`.
func runFix(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("fix", flag.ContinueOnError)
	fl.SetOutput(stderr)
	dry := fl.Bool("dry-run", false, "report the changes without writing them")
	asJSON := fl.Bool("json", false, "print the result as JSON")
	from := fl.String("from", "", "the draft the code was written for (draft-0.1 ... draft-0.6); runs the migration rewriters for later drafts")
	edition := fl.String("edition", "", "the edition to migrate to (default and only one before 1.0: "+check.CurrentEdition+"; it rewrites the old name 2026)")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if *edition != "" {
		if !check.SupportedEdition(*edition) {
			fmt.Fprintf(stderr, "xo fix: unknown edition %q (known: %s)\n", *edition, strings.Join(check.Editions, ", "))
			return 2
		}
	}
	rws, err := fix.Select(*from)
	if err != nil {
		fmt.Fprintf(stderr, "xo fix: %v\n", err)
		return 2
	}
	dirs := fl.Args()
	if len(dirs) == 0 {
		dirs = []string{"."}
	}
	res, err := fix.Run(dirs, fix.Options{Rewriters: rws, DryRun: *dry})
	if err != nil {
		fmt.Fprintf(stderr, "xo fix: %v\n", err)
		return 2
	}
	if *asJSON {
		out := struct {
			DryRun bool `json:"dry_run"`
			*fix.Result
		}{*dry, res}
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			fmt.Fprintf(stderr, "xo fix: %v\n", err)
			return 2
		}
		return 0
	}
	for _, c := range res.Changes {
		fmt.Fprintf(stdout, "%s:%d: %s: %s\n", c.File, c.Line, c.Rule, c.Title)
	}
	verb := "changed"
	if *dry {
		verb = "would change"
	}
	fmt.Fprintf(stdout, "%d change(s), %s %d file(s), %d round(s), %d error(s) left\n", len(res.Changes), verb, len(res.Files), res.Rounds, len(res.Remaining))
	for _, d := range res.Remaining {
		fmt.Fprintln(stderr, diag.Human(d))
	}
	return 0
}
