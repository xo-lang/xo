// Command xo is the xo toolchain. P1 implements `xo parse`, `xo check`,
// `xo build`, `xo run`, `xo test`, `xo bind`, `xo fmt`, `xo vet`,
// `xo query`, `xo doc`, and `xo explain`.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/parser"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

const usage = `usage: xo <command> [arguments]

commands:
  parse [--json] [--ast] [--sig] <files or dirs...>
      Parse .xo files and report syntax diagnostics. Directories are walked
      recursively. --json prints {"files": [...], "diagnostics": [...]} on
      stdout; --ast includes each file's syntax tree; --sig allows function
      declarations without bodies (signature listings). Exit status is 1 when
      any error is reported, 2 on usage errors.
  check [--json] <dirs...>
      Resolve names and type check the modules under each directory (every
      directory holding .xo files is a module; imports are resolved through
      xo.mod and the embedded std). --json prints {"modules": [...],
      "diagnostics": [...]} on stdout. Exit status is 1 when any error is
      reported (warnings such as holes do not count), 2 on usage errors.
  bind [--dir=<project dir>] [--json] <go import paths...>
      Bind Go packages for use go "<path>" (decision 0041): write the Xo
      declaration file and the Go adapter into the binding cache
      ($XO_GOBIND_DIR or <xo.mod dir>/.xo/gobind) and print how much of the
      package mapped. check, build, and test bind missing packages on their
      own. Third party versions are pinned with a line
      "require go <module> <version>" in xo.mod.
  fmt [--check] [--json] [files or dirs...]
      Rewrite .xo files in the one canonical layout (core 10.5; no options)
      and print the names of the files changed. --check changes nothing,
      lists the unformatted files, and exits 1 if there are any. Files with
      syntax errors are reported and left alone (exit 1). Default: .
  vet [--json] [dirs...]
      Advisory checks (never errors, exit 0): the warnings of check, unused
      declared effects (XO0703), x ?? [].f() precedence traps (XO0207), every
      .ignore_err reason (XO0208), use go packages and functions that perform
      go per module (XO1103), dbg(...) left in code (XO0901). Default: .
  query <kind> [--json|--text] [--dir=<dir>] <argument>
      Semantic queries (core 10.3): type <file:line:col>, def <name>,
      callers <module.fn>, effects <module.fn>, holes, api <module path>.
      JSON is the default when stdout is not a terminal.
  doc [--json] [--all] [--dir=<dir>] [module dir or import path]
      Markdown documentation from doc comments and signatures, with the
      example code blocks of each doc comment. Default: the modules under .
  explain [--json] <code>
      Long form explanation of a diagnostic code, with an example and a fix.
  dump [--timeout=5s] <pid>
      Print the task tree of a running Xo program (core 11.4): sends it
      SIGQUIT and prints the dump it writes to $XO_DUMP_DIR or the system
      temporary directory (xo-dump-<pid>.txt). The program goes on running.
`

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "parse":
		return runParse(args[1:], stdout, stderr)
	case "check":
		return runCheck(args[1:], stdout, stderr)
	case "build":
		return runBuild(args[1:], stdout, stderr)
	case "run":
		return runRun(args[1:], stdout, stderr)
	case "test":
		return runTest(args[1:], stdout, stderr)
	case "bind":
		return runBind(args[1:], stdout, stderr)
	case "fmt":
		return runFmt(args[1:], stdout, stderr)
	case "vet":
		return runVet(args[1:], stdout, stderr)
	case "query":
		return runQuery(args[1:], stdout, stderr)
	case "doc":
		return runDoc(args[1:], stdout, stderr)
	case "explain":
		return runExplain(args[1:], stdout, stderr)
	case "dump":
		return runDump(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	}
	fmt.Fprintf(stderr, "xo: unknown command %q\n\n%s", args[0], usage)
	return 2
}

type fileResult struct {
	Path string `json:"path"`
	AST  any    `json:"ast,omitempty"`
}

func runParse(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("parse", flag.ContinueOnError)
	fl.SetOutput(stderr)
	asJSON := fl.Bool("json", false, "print diagnostics (and the AST with --ast) as JSON")
	withAST := fl.Bool("ast", false, "dump the syntax tree as JSON")
	sig := fl.Bool("sig", false, "allow function declarations without bodies")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if fl.NArg() == 0 {
		fmt.Fprintln(stderr, "xo parse: no files or directories given")
		return 2
	}
	files, err := collect(fl.Args())
	if err != nil {
		fmt.Fprintf(stderr, "xo parse: %v\n", err)
		return 2
	}
	var mode parser.Mode
	if *sig {
		mode |= parser.SignatureMode
	}

	var all []diag.Diagnostic
	var results []fileResult
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(stderr, "xo parse: %v\n", err)
			return 2
		}
		f, ds := parser.ParseFile(path, src, mode)
		all = append(all, ds...)
		r := fileResult{Path: path}
		if *withAST {
			r.AST = ast.Dump(f)
		}
		results = append(results, r)
	}

	hasErr := false
	for _, d := range all {
		if d.Severity == diag.Error {
			hasErr = true
		}
	}

	if *asJSON {
		out := struct {
			Files       []fileResult      `json:"files"`
			Diagnostics []diag.Diagnostic `json:"diagnostics"`
		}{results, all}
		if out.Diagnostics == nil {
			out.Diagnostics = []diag.Diagnostic{}
		}
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			fmt.Fprintf(stderr, "xo parse: %v\n", err)
			return 2
		}
	} else {
		if *withAST {
			enc := json.NewEncoder(stdout)
			enc.SetEscapeHTML(false)
			enc.SetIndent("", "  ")
			if err := enc.Encode(results); err != nil {
				fmt.Fprintf(stderr, "xo parse: %v\n", err)
				return 2
			}
		}
		for _, d := range all {
			fmt.Fprintln(stderr, diag.Human(d))
		}
		fmt.Fprintf(stderr, "%d file(s), %d error(s)\n", len(files), len(all))
	}
	if hasErr {
		return 1
	}
	return 0
}

// collect expands directories into the .xo files they contain, sorted.
func collect(paths []string) ([]string, error) {
	var out []string
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			out = append(out, p)
			continue
		}
		err = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(path, ".xo") {
				out = append(out, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

func runCheck(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("check", flag.ContinueOnError)
	fl.SetOutput(stderr)
	asJSON := fl.Bool("json", false, "print diagnostics as JSON")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if fl.NArg() == 0 {
		fmt.Fprintln(stderr, "xo check: no directories given")
		return 2
	}
	res, err := check.Check(fl.Args())
	if err != nil {
		fmt.Fprintf(stderr, "xo check: %v\n", err)
		return 2
	}
	var mods []string
	for _, m := range res.Modules {
		mods = append(mods, m.Path)
	}
	errs, warns := 0, 0
	for _, d := range res.Diagnostics {
		if d.Severity == diag.Error {
			errs++
		} else {
			warns++
		}
	}
	if *asJSON {
		out := struct {
			Modules     []string          `json:"modules"`
			Diagnostics []diag.Diagnostic `json:"diagnostics"`
		}{mods, res.Diagnostics}
		if out.Modules == nil {
			out.Modules = []string{}
		}
		if out.Diagnostics == nil {
			out.Diagnostics = []diag.Diagnostic{}
		}
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			fmt.Fprintf(stderr, "xo check: %v\n", err)
			return 2
		}
	} else {
		for _, d := range res.Diagnostics {
			fmt.Fprintln(stderr, diag.Human(d))
		}
		fmt.Fprintf(stderr, "%d module(s), %d error(s), %d warning(s)\n", len(mods), errs, warns)
	}
	if errs > 0 {
		return 1
	}
	return 0
}
