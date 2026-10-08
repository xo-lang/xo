package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/xo-lang/xo/internal/edit"
)

// runEdit implements `xo edit [--dry-run] [--dir=<dir>] <op> <args...>`
// (core 10.4). It always prints the JSON summary; exit status 1 means the
// edit was rejected and nothing was written.
func runEdit(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("edit", flag.ContinueOnError)
	fl.SetOutput(stderr)
	dry := fl.Bool("dry-run", false, "check the edit and print the summary without writing files")
	dir := fl.String("dir", ".", "directory checked and edited")
	fl.Bool("json", true, "print the summary as JSON (always on)")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	rest := fl.Args()
	if len(rest) == 0 {
		fmt.Fprintln(stderr, "xo edit: missing operation (rename, add-field, change-sig)")
		return 2
	}
	op := rest[0]
	// Flags may also follow the operation.
	if err := fl.Parse(rest[1:]); err != nil {
		return 2
	}
	pos := fl.Args()
	opt := edit.Options{Dirs: []string{*dir}, DryRun: *dry}
	var res *edit.Result
	var err error
	switch op {
	case "rename":
		if len(pos) != 2 {
			fmt.Fprintln(stderr, "usage: xo edit rename <name> <new name>")
			return 2
		}
		res, err = edit.Rename(opt, pos[0], pos[1])
	case "add-field":
		if len(pos) < 2 {
			fmt.Fprintln(stderr, "usage: xo edit add-field <Struct> <name: Type = default>")
			return 2
		}
		res, err = edit.AddField(opt, pos[0], strings.Join(pos[1:], " "))
	case "change-sig":
		if len(pos) < 2 {
			fmt.Fprintln(stderr, `usage: xo edit change-sig <fn> "(params) -> Result ! Err"`)
			return 2
		}
		res, err = edit.ChangeSig(opt, pos[0], strings.Join(pos[1:], " "))
	default:
		fmt.Fprintf(stderr, "xo edit: unknown operation %q (rename, add-field, change-sig)\n", op)
		return 2
	}
	if err != nil {
		fmt.Fprintf(stderr, "xo edit: %v\n", err)
		return 2
	}
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(res); err != nil {
		fmt.Fprintf(stderr, "xo edit: %v\n", err)
		return 2
	}
	if !res.OK {
		return 1
	}
	return 0
}
