// Command xo is the xo toolchain. P1 implements `xo parse`, `xo check`,
// `xo build`, `xo run`, `xo test`, `xo bind`, `xo fmt`, `xo fix`, `xo vet`,
// `xo query`, `xo edit`, `xo doc`, `xo explain`, `xo dump`, `xo replay`, `xo mcp`,
// `xo serve`, and `xo version`.
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
	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/parser"
	"github.com/xo-lang/xo/internal/query"
	"github.com/xo-lang/xo/internal/serve"
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
  check [--json] [--no-gc] <dirs...>
      Resolve names and type check the modules under each directory (every
      directory holding .xo files is a module; imports are resolved through
      xo.mod and the embedded std). --json prints {"modules": [...],
      "diagnostics": [...]} on stdout. --no-gc also runs the check of native
      builds (XO1201 for what they cannot build yet, or ever; decision 0083).
      Exit status is 1 when any error is reported (warnings such as holes do
      not count), 2 on usage errors.
  build [--targets=all|os/arch,...] [--backend=go|llvm|arm64] [--no-gc] [--no-mono] [--no-arenas] [--tzdata=embedded|system] [--ci] [-o out] <dirs...>
      Compile to static binaries (default: the host). --backend=llvm uses the
      LLVM prototype (a core subset, needs clang); --backend=arm64 the native
      AArch64 backend (the same subset, needs cc; linux/arm64 is emitted as
      assembly when it is not the host). --no-gc (also on run and test)
      builds without a garbage collector: arm64 on arm64 hosts, llvm
      otherwise (--backend may name one); every construct, std API, or
      effect the native backends do not support is reported as XO1201
      before code generation (decision 0083). Request scoped arenas for HTTP
      handlers are on by default (decision 0066); --no-arenas (also on run
      and test) turns them off. --ci fails before building when a .xo file
      is not formatted (core 10.5) or dbg(...) is left in code (core 11.4).
      std/time zone data is embedded per use (decision 0094): the zones
      named by constants, or the whole database (with a note naming the
      call) for a name known only at run time; --tzdata=system (also on run
      and test) reads the host's database instead.
  run [--seed=N] [--backend=go|llvm|arm64] [--no-gc] <dir> [-- args]
      Build for the host and run. --seed makes task interleavings
      reproducible. Recording is described under run --record below.
  test [--seed=N] [--explore=N] [--run=text] [--race] [--real] [--cases=N] [--json] [-v] [--no-gc] <dirs...>
      Run test blocks, property tests, and contract fuzzing on the
      deterministic scheduler. Shrunk contract failures are written to the
      module's regressions_test.xo.
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
  fix [--dry-run] [--json] [--from=draft-0.N] [--edition=YYYY] [dirs...]
      Apply the machine fixes of check diagnostics, round after round,
      until none apply, then reformat the changed files and print what
      changed. --from runs the migration rewriters for every breaking
      change after that draft first (draft-0.1: ok/err/some/none case,
      scope timeout, {x} interpolation, let _ =, Map index reads).
      --dry-run writes nothing. Exit status 0 even when errors remain
      (they are listed); 2 on usage errors. Default: .
  vet [--json] [dirs...]
      Advisory checks (never errors, exit 0): the warnings of check, unused
      declared effects (XO0703), x ?? [].f() precedence traps (XO0207), every
      .ignore_err reason (XO0208), use go packages and functions that perform
      go per module (XO1103), dbg(...) left in code (XO0901). Default: .
  query <kind> [--json|--text] [--dir=<dir>] <argument>
      Semantic queries (core 10.3): type <file:line:col>, def <name>,
      callers <module.fn>, effects <module.fn>, holes, api <module path>.
      JSON is the default when stdout is not a terminal.
  edit [--dry-run] [--dir=<dir>] <op> <args...>
      Semantic edits (core 10.4) through the checker's resolved symbols:
        rename <name> <new name>           function, method, or constant
        add-field <Struct> <name: Type = default>
        change-sig <fn> "(params) -> Result ! Err"
      Every reference is rewritten at once, and the edit is rejected
      (nothing written, exit 1) unless the program checks clean before and
      after it; the JSON summary lists the files, the diagnostics, or the
      call sites that blocked it. Default dir: .
  doc [--json] [--all] [--dir=<dir>] [module dir or import path]
      Markdown documentation from doc comments and signatures, with the
      example code blocks of each doc comment. Default: the modules under .
  explain [--json] <code>
      Long form explanation of a diagnostic code, with an example and a fix.
  dump [--timeout=5s] <pid>
      Print the task tree of a running Xo program (core 11.4): sends it
      SIGQUIT and prints the dump it writes to $XO_DUMP_DIR or the system
      temporary directory (xo-dump-<pid>.txt). The program goes on running.
  run --record=<trace.xotrace> [dir] [-- args...]
      Run with every effect (clock, rand, env, stdin, fs, net, http client
      responses, server requests, signals) and every scheduling decision
      recorded into the trace (core 11.3). XO_RECORD=<file> does the same
      for any built program; XO_RING=N keeps per request ring buffers of
      the last N effects for crash bundles.
  replay [--dir=<module dir>] <trace.xotrace | crash.xocrash>
      Rebuild the recorded program and rerun it offline with every effect
      result and interleaving taken from the trace: same output, same
      fault. A divergence (the program asks for another effect than the
      trace recorded) is reported with both locations, exit status 3.
      Secrets are redacted in traces; a replay takes them from the
      environment (the recorded variable, or XO_REPLAY_SECRET_<n>).
  mcp
      Model Context Protocol server on stdin/stdout (JSON-RPC 2.0, one
      message per line) with the tools check, query, edit, explain, fmt, test,
      and build, each returning compact JSON.
  serve [--socket=path] [stop|status]
      Keep checked programs in memory and answer xo check and xo query over
      a unix socket (default $XO_SERVE_SOCKET or <user cache dir>/xo/
      serve.sock). Results are cached per directory set and rechecked when
      any source file's modification time or size changes. check and query
      use a running server automatically; XO_NO_SERVE=1 turns that off.
      stop shuts the server down; status prints its cache counters.
  get [--dir=<dir>] [--json] <module path>[@version|@latest|@none]...
      Require a module (decision 0086): the latest vX.Y.Z tag unless a
      version is given; @none removes the requirement. Fetches with git
      into the module cache ($XO_MODCACHE or <user cache dir>/xo/mod),
      resolves the graph by minimal version selection, and rewrites xo.mod
      and xo.lock. Module paths are git locations; XO_GIT_BASE=<dir or URL>
      or XO_GIT_REWRITE=<prefix>=<url>,... point them elsewhere.
  mod tidy [--dir=<dir>] [--json]
      Require every imported module, drop requirements nothing imports
      (unless they raise a version the graph selects), and rewrite xo.lock
      with exactly the hashes the build uses. Ignores xo.work and vendor/.
  mod clean
      Remove the module cache (it is read only, so rm -rf fails).
  vendor [--dir=<dir>] [--json]
      Copy every selected module version into vendor/ with
      vendor/modules.txt; builds then use vendor/ only (no cache, no
      network), verified against xo.lock.
  Every command accepts --offline (XO_OFFLINE=1: never fetch; a missing
  module is XO1303) and --workspace=off (XO_WORK=off: ignore xo.work).
  An xo.work file (xo 2026 / use ./dir lines) builds the modules it uses
  from their directories, overriding requirements; check, test, query, and
  edit then take every module of the workspace as one program, and
  build --ci fails (XO1308) while it overrides a required module.
  version
      Print the toolchain version, the source revision, and the host target.
`

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	rest, restore, err := moduleFlags(args[1:])
	defer restore()
	if err != nil {
		fmt.Fprintf(stderr, "xo %s: %v\n", args[0], err)
		return 2
	}
	args = append([]string{args[0]}, rest...)
	switch args[0] {
	case "get":
		return runGet(args[1:], stdout, stderr)
	case "mod":
		return runMod(args[1:], stdout, stderr)
	case "vendor":
		return runVendor(args[1:], stdout, stderr)
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
	case "fix":
		return runFix(args[1:], stdout, stderr)
	case "query":
		return runQuery(args[1:], stdout, stderr)
	case "edit":
		return runEdit(args[1:], stdout, stderr)
	case "doc":
		return runDoc(args[1:], stdout, stderr)
	case "explain":
		return runExplain(args[1:], stdout, stderr)
	case "dump":
		return runDump(args[1:], stdout, stderr)
	case "replay":
		return runReplay(args[1:], stdout, stderr)
	case "mcp":
		return runMCP(args[1:], os.Stdin, stdout, stderr)
	case "serve":
		return runServe(args[1:], stdout, stderr)
	case "version", "--version":
		return runVersion(args[1:], stdout, stderr)
	case "lexdump": // hidden: canonical token dump, compared with selfhost/lexer
		return runLexDump(args[1:], stdout, stderr)
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
	noGC := false
	for _, a := range args {
		if a == "--no-gc" || a == "-no-gc" || a == "--no-gc=true" || a == "-no-gc=true" {
			noGC = true
		}
	}
	if !noGC {
		if code, ok := serve.Forward(append([]string{"check"}, args...), stdout, stderr); ok {
			return code
		}
	}
	return runCheckWith(args, query.Load, stdout, stderr)
}

// runCheckWith is `xo check` with the given session loader (in process, or
// the cache of `xo serve`).
func runCheckWith(args []string, load loader, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("check", flag.ContinueOnError)
	fl.SetOutput(stderr)
	asJSON := fl.Bool("json", false, "print diagnostics as JSON")
	noGC := fl.Bool("no-gc", false, "also run the no GC check of native builds (XO1201, decision 0083)")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if fl.NArg() == 0 {
		fmt.Fprintln(stderr, "xo check: no directories given")
		return 2
	}
	var res *check.Result
	var ds []diag.Diagnostic
	if *noGC {
		r, all, err := build.CheckNoGCDirs(fl.Args(), true)
		if err != nil {
			fmt.Fprintf(stderr, "xo check: %v\n", err)
			return 2
		}
		res, ds = r, all
	} else {
		sess, err := load(fl.Args())
		if err != nil {
			fmt.Fprintf(stderr, "xo check: %v\n", err)
			return 2
		}
		res, ds = sess.Res, sess.Res.Diagnostics
	}
	var mods []string
	for _, m := range res.Modules {
		mods = append(mods, m.Path)
	}
	errs, warns := 0, 0
	for _, d := range ds {
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
		}{mods, ds}
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
		for _, d := range ds {
			fmt.Fprintln(stderr, diag.Human(d))
		}
		fmt.Fprintf(stderr, "%d module(s), %d error(s), %d warning(s)\n", len(mods), errs, warns)
	}
	if errs > 0 {
		return 1
	}
	return 0
}
