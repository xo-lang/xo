package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/doc"
	"github.com/xo-lang/xo/internal/format"
	"github.com/xo-lang/xo/internal/query"
	"github.com/xo-lang/xo/internal/serve"
	"github.com/xo-lang/xo/internal/vet"
)

// isTerminal reports whether w is a terminal (a character device).
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// ---------------------------------------------------------------------------
// fmt

// collectFmt expands directories into the .xo files under them, skipping
// dot directories (such as the `.xo` binding cache).
func collectFmt(paths []string) ([]string, error) {
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
			if d.IsDir() {
				if path != p && strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(path, ".xo") {
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

type fmtFile struct {
	Path    string `json:"path"`
	Changed bool   `json:"changed"`
}

func runFmt(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("fmt", flag.ContinueOnError)
	fl.SetOutput(stderr)
	checkOnly := fl.Bool("check", false, "list unformatted files and exit 1 if any; change nothing")
	asJSON := fl.Bool("json", false, "print the result as JSON")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	paths := fl.Args()
	if len(paths) == 0 {
		paths = []string{"."}
	}
	files, err := collectFmt(paths)
	if err != nil {
		fmt.Fprintf(stderr, "xo fmt: %v\n", err)
		return 2
	}
	results := []fmtFile{}
	diags := []diag.Diagnostic{}
	changed, failed := 0, 0
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(stderr, "xo fmt: %v\n", err)
			return 2
		}
		out, err := format.Source(path, src)
		if err != nil {
			var pe *format.ParseError
			if errors.As(err, &pe) {
				diags = append(diags, pe.Diagnostics...)
				failed++
				continue
			}
			fmt.Fprintf(stderr, "xo fmt: %s: %v\n", path, err)
			return 2
		}
		r := fmtFile{Path: path, Changed: string(out) != string(src)}
		results = append(results, r)
		if !r.Changed {
			continue
		}
		changed++
		if !*checkOnly {
			info, err := os.Stat(path)
			if err != nil {
				fmt.Fprintf(stderr, "xo fmt: %v\n", err)
				return 2
			}
			if err := os.WriteFile(path, out, info.Mode().Perm()); err != nil {
				fmt.Fprintf(stderr, "xo fmt: %v\n", err)
				return 2
			}
		}
		if !*asJSON {
			fmt.Fprintln(stdout, path)
		}
	}
	if *asJSON {
		if err := writeJSON(stdout, struct {
			Files       []fmtFile         `json:"files"`
			Diagnostics []diag.Diagnostic `json:"diagnostics"`
		}{results, diags}); err != nil {
			fmt.Fprintf(stderr, "xo fmt: %v\n", err)
			return 2
		}
	} else {
		for _, d := range diags {
			fmt.Fprintln(stderr, diag.Human(d))
		}
		if failed > 0 {
			fmt.Fprintf(stderr, "xo fmt: %d file(s) not formatted because of syntax errors\n", failed)
		}
	}
	if failed > 0 || (*checkOnly && changed > 0) {
		return 1
	}
	return 0
}

// ---------------------------------------------------------------------------
// vet

func runVet(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("vet", flag.ContinueOnError)
	fl.SetOutput(stderr)
	asJSON := fl.Bool("json", false, "print the findings as JSON")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	dirs := fl.Args()
	if len(dirs) == 0 {
		dirs = []string{"."}
	}
	r, err := vet.Vet(dirs)
	if err != nil {
		fmt.Fprintf(stderr, "xo vet: %v\n", err)
		return 2
	}
	if *asJSON {
		out := struct {
			Modules     []string          `json:"modules"`
			Diagnostics []diag.Diagnostic `json:"diagnostics"`
			CheckErrors int               `json:"check_errors"`
		}{r.Modules, r.Diagnostics, r.CheckErrors}
		if out.Modules == nil {
			out.Modules = []string{}
		}
		if out.Diagnostics == nil {
			out.Diagnostics = []diag.Diagnostic{}
		}
		if err := writeJSON(stdout, out); err != nil {
			fmt.Fprintf(stderr, "xo vet: %v\n", err)
			return 2
		}
		return 0
	}
	warns, notes := 0, 0
	for _, d := range r.Diagnostics {
		fmt.Fprintln(stdout, diag.Human(d))
		if d.Severity == diag.Note {
			notes++
		} else {
			warns++
		}
	}
	fmt.Fprintf(stderr, "%d module(s), %d warning(s), %d note(s)\n", len(r.Modules), warns, notes)
	if r.CheckErrors > 0 {
		fmt.Fprintf(stderr, "xo vet: the program has %d check error(s), not repeated here; run `xo check`\n", r.CheckErrors)
	}
	return 0
}

// ---------------------------------------------------------------------------
// query

const queryUsage = `usage: xo query <kind> [--json|--text] [--dir=<dir>] <argument>

kinds:
  type <file:line:col>     type of the innermost expression or binding there
  def <name>               declaration of name, Type.member, module.name, module.Type.member
  callers <module.fn>      calls of the function and uses of it as a value
  effects <module.fn>      effects the function performs, with the call chain per effect
  holes                    holes (???) with their expected types
  api <module path>        exported surface of a module (import path or directory)
  imports <package>        the package's use lines, with any deny rule that forbids one
  deps <package>           imports, transitively (std and Go packages are not followed)
  hash <name>              content hash of a definition of the module (name, Type.method,
                           pkg.name, or <package path>.name): local names erased,
                           referenced definitions by their hashes (core 4.3)

--dir (default .) is the directory checked. JSON is the default when stdout
is not a terminal.
`

func runQuery(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		fwd := append([]string{"query"}, args...)
		if isTerminal(stdout) && !hasFormatFlag(args) {
			fwd = append(fwd, "--text")
		}
		if code, ok := serve.Forward(fwd, stdout, stderr); ok {
			return code
		}
	}
	return runQueryWith(args, query.Load, stdout, stderr)
}

// hasFormatFlag reports whether a query command line picks JSON or text.
func hasFormatFlag(args []string) bool {
	for _, a := range args {
		switch a {
		case "--json", "-json", "--text", "-text":
			return true
		}
	}
	return false
}

// runQueryWith is `xo query` with the given session loader.
func runQueryWith(args []string, load loader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, queryUsage)
		return 2
	}
	kind := args[0]
	asJSON := !isTerminal(stdout)
	dir := "."
	var rest []string
	for _, a := range args[1:] {
		switch {
		case a == "--json" || a == "-json":
			asJSON = true
		case a == "--text" || a == "-text":
			asJSON = false
		case strings.HasPrefix(a, "--dir="):
			dir = strings.TrimPrefix(a, "--dir=")
		case strings.HasPrefix(a, "-") && a != "-":
			fmt.Fprintf(stderr, "xo query: unknown flag %s\n\n%s", a, queryUsage)
			return 2
		default:
			rest = append(rest, a)
		}
	}
	want := 1
	if kind == "holes" {
		want = 0
	}
	if len(rest) != want {
		fmt.Fprintf(stderr, "xo query %s: want %d argument(s)\n\n%s", kind, want, queryUsage)
		return 2
	}
	dirs := []string{dir}
	var file string
	var line, col int
	if kind == "type" {
		var err error
		file, line, col, err = parsePos(rest[0])
		if err != nil {
			fmt.Fprintf(stderr, "xo query type: %v\n", err)
			return 2
		}
		if !within(file, dir) {
			dirs = append(dirs, filepath.Dir(file))
		}
	}
	switch kind {
	case "hash":
		// syntactic: no check run (decision 0125)
		return queryHash(dir, rest[0], asJSON, stdout, stderr)
	case "type", "def", "callers", "effects", "holes", "api", "imports", "deps":
	default:
		fmt.Fprintf(stderr, "xo query: unknown kind %q\n\n%s", kind, queryUsage)
		return 2
	}
	s, err := load(dirs)
	if err != nil {
		fmt.Fprintf(stderr, "xo query: %v\n", err)
		return 2
	}
	var res any
	var text func() string
	switch kind {
	case "type":
		r, e := s.Type(file, line, col)
		res, err = r, e
		text = func() string {
			out := fmt.Sprintf("%s: `%s`: %s", r.Pos, r.Expr, r.Type)
			if r.Natural != "" {
				out += fmt.Sprintf(" (converted from %s)", r.Natural)
			}
			return out + "\n"
		}
	case "def":
		r, e := s.Def(rest[0])
		res, err = r, e
		text = func() string {
			var b strings.Builder
			for _, d := range r {
				fmt.Fprintf(&b, "%s: %s %s (module %s)\n", d.Pos, d.Kind, d.Name, d.Module)
				if d.Doc != "" {
					fmt.Fprintf(&b, "%s\n", prefixLines(d.Doc, "/// "))
				}
				fmt.Fprintf(&b, "%s\n", d.Signature)
			}
			return b.String()
		}
	case "callers":
		r, e := s.Callers(rest[0])
		res, err = r, e
		text = func() string {
			var b strings.Builder
			for _, c := range r.Callers {
				fmt.Fprintf(&b, "%s: %s in %s.%s\n", c.Pos, c.Kind, c.Module, c.Caller)
			}
			fmt.Fprintf(&b, "%d reference(s) to %s\n", len(r.Callers), r.Target)
			return b.String()
		}
	case "effects":
		r, e := s.Effects(rest[0])
		res, err = r, e
		text = func() string {
			var b strings.Builder
			switch {
			case r.Inferred:
				fmt.Fprintf(&b, "%s: effects inferred\n", r.Target)
			case len(r.Declared) == 0:
				fmt.Fprintf(&b, "%s: declared pure\n", r.Target)
			default:
				fmt.Fprintf(&b, "%s: declares uses %s\n", r.Target, strings.Join(r.Declared, ", "))
			}
			if len(r.Performed) == 0 {
				b.WriteString("performs no effects\n")
			}
			for _, ef := range r.Performed {
				fmt.Fprintf(&b, "performs %s\n", ef.Effect)
				for _, st := range ef.Chain {
					fmt.Fprintf(&b, "  %s: %s\n", st.Pos, st.Message)
				}
			}
			return b.String()
		}
	case "holes":
		r := s.Holes()
		res = r
		text = func() string {
			var b strings.Builder
			for _, d := range r {
				fmt.Fprintln(&b, diag.Human(d))
			}
			fmt.Fprintf(&b, "%d hole(s)\n", len(r))
			return b.String()
		}
	case "imports", "deps":
		r, e := s.Imports(rest[0], kind == "deps")
		res, err = r, e
		text = func() string {
			var b strings.Builder
			for _, e := range r.Imports {
				fmt.Fprintf(&b, "%s: %s uses %s", e.Pos, e.From, e.Path)
				if e.Missing {
					b.WriteString(" (not found)")
				}
				if e.Denied != "" {
					b.WriteString(" (denied, XO1312)")
				}
				b.WriteString("\n")
			}
			fmt.Fprintf(&b, "%d use line(s), %d package(s)\n", len(r.Imports), len(r.Packages))
			return b.String()
		}
	case "api":
		r, e := s.API(rest[0])
		res, err = r, e
		text = func() string {
			var b strings.Builder
			fmt.Fprintf(&b, "module %s\n", r.Module)
			for _, d := range r.Items {
				fmt.Fprintf(&b, "\n%s\n", d.Signature)
			}
			return b.String()
		}
	}
	if err != nil {
		fmt.Fprintf(stderr, "xo query %s: %v\n", kind, err)
		return 1
	}
	if asJSON {
		if err := writeJSON(stdout, res); err != nil {
			fmt.Fprintf(stderr, "xo query: %v\n", err)
			return 2
		}
		return 0
	}
	fmt.Fprint(stdout, text())
	return 0
}

func prefixLines(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(prefix+l, " ")
	}
	return strings.Join(lines, "\n")
}

// parsePos splits `file:line:col`.
func parsePos(s string) (string, int, int, error) {
	i := strings.LastIndexByte(s, ':')
	if i < 0 {
		return "", 0, 0, fmt.Errorf("want file:line:col, got %q", s)
	}
	j := strings.LastIndexByte(s[:i], ':')
	if j < 0 {
		return "", 0, 0, fmt.Errorf("want file:line:col, got %q", s)
	}
	line, err1 := strconv.Atoi(s[j+1 : i])
	col, err2 := strconv.Atoi(s[i+1:])
	if err1 != nil || err2 != nil || line < 1 || col < 1 {
		return "", 0, 0, fmt.Errorf("want file:line:col with positive numbers, got %q", s)
	}
	return s[:j], line, col, nil
}

// within reports whether path lies under dir.
func within(path, dir string) bool {
	ap, err1 := filepath.Abs(path)
	ad, err2 := filepath.Abs(dir)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(ad, ap)
	return err == nil && !strings.HasPrefix(rel, "..")
}

// ---------------------------------------------------------------------------
// doc

func runDoc(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("doc", flag.ContinueOnError)
	fl.SetOutput(stderr)
	asJSON := fl.Bool("json", false, "print the documentation as JSON")
	all := fl.Bool("all", false, "include declarations that are not pub")
	dir := fl.String("dir", ".", "directory checked when the module is given by import path")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if fl.NArg() > 1 {
		fmt.Fprintln(stderr, "xo doc: at most one module")
		return 2
	}
	target := "."
	if fl.NArg() == 1 {
		target = fl.Arg(0)
	}
	var docs []*doc.Module
	if info, err := os.Stat(target); err == nil && info.IsDir() {
		s, err := query.LoadExact([]string{target})
		if err != nil {
			fmt.Fprintf(stderr, "xo doc: %v\n", err)
			return 2
		}
		for _, m := range s.Res.Modules {
			docs = append(docs, doc.Build(s, m, *all))
		}
		if len(docs) == 0 {
			fmt.Fprintf(stderr, "xo doc: no .xo files under %s\n", target)
			return 1
		}
	} else {
		s, err := query.Load([]string{*dir})
		if err != nil {
			fmt.Fprintf(stderr, "xo doc: %v\n", err)
			return 2
		}
		m := s.ModuleByPath(target)
		if m == nil {
			fmt.Fprintf(stderr, "xo doc: no module %q under %s\n", target, *dir)
			return 1
		}
		docs = append(docs, doc.Build(s, m, *all))
	}
	if *asJSON {
		if err := writeJSON(stdout, docs); err != nil {
			fmt.Fprintf(stderr, "xo doc: %v\n", err)
			return 2
		}
		return 0
	}
	for i, d := range docs {
		if i > 0 {
			fmt.Fprintln(stdout)
		}
		fmt.Fprint(stdout, d.Markdown())
	}
	return 0
}

// ---------------------------------------------------------------------------
// explain

func runExplain(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("explain", flag.ContinueOnError)
	fl.SetOutput(stderr)
	asJSON := fl.Bool("json", false, "print the explanation as JSON")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if fl.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: xo explain [--json] <code>   (for example XO0201)")
		return 2
	}
	code := diag.NormalizeCode(fl.Arg(0))
	e, ok := diag.Explanations[code]
	if !ok {
		fmt.Fprintf(stderr, "xo explain: unknown diagnostic code %q\n", fl.Arg(0))
		return 1
	}
	if *asJSON {
		if err := writeJSON(stdout, map[string]string{
			"code": string(code), "severity": string(diag.SeverityOf(code)), "title": e.Title,
			"summary": diag.Descriptions[code], "detail": e.Detail, "example": e.Example, "fix": e.Fix,
		}); err != nil {
			fmt.Fprintf(stderr, "xo explain: %v\n", err)
			return 2
		}
		return 0
	}
	text, _ := diag.Explain(string(code))
	fmt.Fprint(stdout, text)
	return 0
}
