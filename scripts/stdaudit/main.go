// Command stdaudit lists Go's exported API per package that std-coverage.md
// maps (the function-level audit, lead order 2026-11-10), for comparison with
// Xo's std. This first stage prints the Go side:
//
//	go run ./scripts/stdaudit -go [-md FILE] [PKG...]
//
// one line a symbol: `PKG KIND NAME` (KIND: func, method, type, const, var;
// a method's NAME is Type.Method). The packages are the rows of the
// "Go packages" table of std-coverage.md (the first column), or PKG
// arguments. Test files, internal and vendor packages are skipped.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/doc"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// packages returns the Go package paths in the table under "## Go packages".
func packages(md string) ([]string, error) {
	rows, err := pkgRows(md)
	var out []string
	for _, r := range rows {
		out = append(out, r.pkg)
	}
	return out, err
}

// pkgRow is one package of the coverage table with its row's columns.
type pkgRow struct{ pkg, module, status, note string }

func pkgRows(md string) ([]pkgRow, error) {
	f, err := os.Open(md)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []pkgRow
	in := false
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		l := sc.Text()
		if strings.HasPrefix(l, "## ") {
			in = l == "## Go packages"
			continue
		}
		if !in || !strings.HasPrefix(l, "| ") {
			continue
		}
		cols := strings.Split(l, "|")
		cell := strings.TrimSpace(cols[1])
		var module, status, note string
		if len(cols) > 4 {
			module, status = strings.TrimSpace(cols[2]), strings.TrimSpace(cols[3])
			note = strings.TrimSpace(cols[4])
		}
		if cell == "" || cell == "Go package" || strings.HasPrefix(cell, "---") {
			continue
		}
		prefix := ""
		for _, p := range strings.Split(cell, ",") {
			p = strings.Trim(strings.TrimSpace(p), "`")
			// "net/http (server)" names a package with a note
			if i := strings.Index(p, " "); i > 0 {
				p = p[:i]
			}
			// "debug/dwarf, elf, gosym": the later names sit beside the first
			if prefix != "" && !strings.Contains(p, "/") && isDir(prefix+"/"+p) && !isDir(p) {
				p = prefix + "/" + p
			}
			if i := strings.LastIndex(p, "/"); i > 0 && prefix == "" {
				prefix = p[:i]
			}
			if p != "" && !strings.ContainsAny(p, "*") && isDir(p) {
				out = append(out, pkgRow{p, module, status, note})
			}
		}
	}
	return out, sc.Err()
}

func isDir(pkg string) bool {
	st, err := os.Stat(filepath.Join(runtime.GOROOT(), "src", filepath.FromSlash(pkg)))
	return err == nil && st.IsDir()
}

// api lists the exported symbols of package pkg of GOROOT.
var ctxt = func() build.Context {
	c := build.Default
	c.GOOS, c.GOARCH, c.CgoEnabled = "linux", "arm64", false
	return c
}()

func api(pkg string) ([]string, error) {
	dir := filepath.Join(runtime.GOROOT(), "src", filepath.FromSlash(pkg))
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		n := fi.Name()
		if strings.HasSuffix(n, "_test.go") {
			return false
		}
		// one build context (linux/arm64, cgo off): syscall and os
		// files of every OS would list each platform's constants
		ok, err := ctxt.MatchFile(dir, n)
		return err == nil && ok
	}, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	var out []string
	for name, p := range pkgs {
		if strings.HasSuffix(name, "_test") || name == "main" {
			continue
		}
		d := doc.New(p, pkg, 0) // exported only
		for _, c := range d.Consts {
			for _, n := range c.Names {
				out = append(out, pkg+" const "+n)
			}
		}
		for _, v := range d.Vars {
			for _, n := range v.Names {
				out = append(out, pkg+" var "+n)
			}
		}
		for _, f := range d.Funcs {
			out = append(out, pkg+" func "+f.Name)
		}
		for _, t := range d.Types {
			out = append(out, pkg+" type "+t.Name)
			for _, c := range t.Consts {
				for _, n := range c.Names {
					out = append(out, pkg+" const "+n)
				}
			}
			for _, v := range t.Vars {
				for _, n := range v.Names {
					out = append(out, pkg+" var "+n)
				}
			}
			for _, f := range t.Funcs {
				out = append(out, pkg+" func "+f.Name)
			}
			for _, m := range t.Methods {
				if ast.IsExported(m.Name) {
					out = append(out, pkg+" method "+t.Name+"."+m.Name)
				}
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

// xoSide prints Xo's surface, one line a symbol: `NAME<TAB>DOC`, where
// NAME is module.name (std/uuid.parse) or Type.method (builtin types, from
// `xo doc`), and DOC is the doc comment's first line (it often names Go's
// function: "Go's strconv.ParseInt", which seeds the mapping).
func xoSide(xo, stdDir string) error {
	builtins := []string{"Int", "Float", "Bool", "Str", "Bytes", "List", "Map", "Set", "Option", "Result", "Rune", "Byte", "Time", "Duration"}
	for _, t := range builtins {
		out, err := exec.Command(xo, "doc", t).Output()
		if err != nil {
			continue
		}
		for _, l := range strings.Split(string(out), "\n") {
			if !strings.HasPrefix(l, "  "+t+".") {
				continue
			}
			name, rest, _ := strings.Cut(strings.TrimSpace(l), "  ")
			_, doc, _ := strings.Cut(rest, "//")
			fmt.Printf("%s\t%s\n", name, strings.TrimSpace(doc))
		}
	}
	files, _ := filepath.Glob(filepath.Join(stdDir, "*.xo"))
	sort.Strings(files)
	for _, f := range files {
		m := strings.TrimSuffix(filepath.Base(f), ".xo")
		if strings.HasSuffix(m, "_test") {
			continue
		}
		out, err := exec.Command(xo, "symbols", "--module=std/"+m, "--json").Output()
		if err != nil {
			continue
		}
		var res struct {
			Symbols []struct{ Name, Kind, Doc string }
		}
		if json.Unmarshal(out, &res) != nil {
			continue
		}
		for _, s := range res.Symbols {
			if s.Kind == "field" {
				continue
			}
			doc, _, _ := strings.Cut(s.Doc, "\n")
			fmt.Printf("std/%s.%s\t%s\n", m, s.Name, doc)
		}
	}
	return nil
}

// xoHas reports whether name is an Xo symbol or a module/type prefix of one.
func xoHas(xo map[string]bool, name string) bool {
	if xo[name] || xo["std/prelude."+name] {
		return true
	}
	if strings.HasPrefix(name, "==") || name == "Display" {
		return true // operators and prelude interfaces are not symbols of the listing
	}
	for k := range xo {
		if strings.HasPrefix(k, name+".") {
			return true
		}
	}
	return false
}

// snake converts Go's CamelCase to Xo's snake_case: HasPrefix -> has_prefix,
// ParseURL -> parse_url, IPv4 -> ipv4.
func snake(s string) string {
	var out []rune
	rs := []rune(s)
	for i, r := range rs {
		if r >= 'A' && r <= 'Z' {
			prevLower := i > 0 && (rs[i-1] >= 'a' && rs[i-1] <= 'z' || rs[i-1] >= '0' && rs[i-1] <= '9')
			nextLower := i+1 < len(rs) && rs[i+1] >= 'a' && rs[i+1] <= 'z'
			if i > 0 && (prevLower || (rs[i-1] >= 'A' && rs[i-1] <= 'Z' && nextLower)) {
				out = append(out, '_')
			}
			out = append(out, r+('a'-'A'))
		} else {
			out = append(out, r)
		}
	}
	return string(out)
}

// renames are Go names Xo spells differently, per Go package.
var renames = map[string]string{
	"strings.HasPrefix": "starts_with", "strings.HasSuffix": "ends_with",
	"strings.ToLower": "to_lower", "strings.ToUpper": "to_upper",
	"strings.TrimSpace": "trim", "strings.TrimPrefix": "trim_prefix",
	"bytes.HasPrefix": "starts_with", "bytes.HasSuffix": "ends_with",
	"strconv.Itoa": "to_str", "strconv.Atoi": "parse",
	"slices.Index": "index_of", "slices.Contains": "contains",
}

// typeTargets are the Xo prefixes a Go package's top-level functions map
// onto when Xo spells them as methods of a builtin type.
var typeTargets = map[string][]string{
	"strings": {"Str"}, "bytes": {"Bytes"}, "strconv": {"Int", "Float", "Str", "Bool"},
	"unicode": {"Rune"}, "unicode/utf8": {"Rune", "Str"}, "slices": {"List"}, "maps": {"Map"},
	"sort": {"List"}, "math": {"Int", "Float"}, "math/bits": {"Int"}, "cmp": {"Int"},
}

// modName is the std module that follows Go package pkg.
func modName(pkg string) string {
	switch pkg {
	case "net/http":
		return "http"
	case "encoding/json":
		return "json"
	case "math/rand", "math/rand/v2":
		return "rand"
	}
	return pkg[strings.LastIndex(pkg, "/")+1:]
}

// seedMatch looks for the Xo name a Go symbol obviously maps to: the
// snake_case spelling in the package's builtin type or std module.
func seedMatch(xo map[string]bool, pkg, kind, name string) string {
	var cands []string
	short := pkg[strings.LastIndex(pkg, "/")+1:]
	n := snake(name)
	if r, ok := renames[short+"."+name]; ok {
		n = r
	}
	mod := "std/" + modName(pkg)
	if kind == "method" {
		typ, meth, _ := strings.Cut(name, ".")
		cands = append(cands, mod+"."+typ+"."+snake(meth))
		for _, t := range typeTargets[pkg] {
			cands = append(cands, t+"."+snake(meth))
		}
	} else {
		for _, t := range typeTargets[pkg] {
			cands = append(cands, t+"."+n, "std/prelude."+t+"."+n)
		}
		cands = append(cands, mod+"."+n, mod+"."+name, "std/prelude."+n)
		if kind == "const" || kind == "var" {
			cands = append(cands, mod+"."+strings.ToUpper(snake(name)))
		}
	}
	for _, c := range cands {
		if xo[c] {
			return c
		}
	}
	return ""
}

// report joins the two sides: a Go symbol is covered when map.txt maps it
// (`pkg.Name = xo name`) or skips it (`pkg.Name = not planned: reason`), or
// when an Xo doc comment names it (`Go's pkg.Name`, `strconv.ParseInt`,
// or `Go's Name` in the module of the same package); else unmapped.
// Lines of the output: `STATUS<TAB>pkg kind name<TAB>detail`.
func report(goFile, xoFile, mapFile, md string) error {
	rows, _ := pkgRows(md)
	notPlanned := map[string]string{}
	for _, r := range rows {
		if strings.HasPrefix(r.status, "not planned") {
			notPlanned[r.pkg] = "not planned: " + r.note
		}
	}
	read := func(f string) ([]string, error) {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		return strings.Split(strings.TrimRight(string(b), "\n"), "\n"), nil
	}
	gl, err := read(goFile)
	if err != nil {
		return err
	}
	xl, err := read(xoFile)
	if err != nil {
		return err
	}
	mapped := map[string]string{}
	if mapFile != "" {
		ml, err := read(mapFile)
		if err != nil {
			return err
		}
		for _, l := range ml {
			if l = strings.TrimSpace(l); l == "" || strings.HasPrefix(l, "#") {
				continue
			}
			k, v, ok := strings.Cut(l, "=")
			if ok {
				mapped[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
	}
	docs := strings.Join(xl, "\n")
	xoset := map[string]bool{}
	for _, l := range xl {
		n, _, _ := strings.Cut(l, "\t")
		xoset[n] = true
	}
	for _, l := range gl {
		f := strings.Fields(l)
		if len(f) != 3 {
			continue
		}
		pkg, kind, name := f[0], f[1], f[2]
		short := pkg[strings.LastIndex(pkg, "/")+1:]
		key := short + "." + name
		if mapped[pkg+"."+name] != "" {
			key = pkg + "." + name
		}
		switch {
		case mapped[key] == "" && notPlanned[pkg] != "":
			fmt.Printf("skipped\t%s\t%s\n", l, notPlanned[pkg])
		case strings.HasPrefix(mapped[key], "gap"):
			fmt.Printf("gap\t%s\t%s\n", l, mapped[key])
		case mapped[key] != "" && strings.HasPrefix(mapped[key], "not planned"):
			fmt.Printf("skipped\t%s\t%s\n", l, mapped[key])
		case mapped[key] != "" && !strings.Contains(mapped[key], " ") && !xoHas(xoset, mapped[key]):
			fmt.Printf("stale\t%s\t%s\n", l, mapped[key])
		case mapped[key] != "":
			fmt.Printf("mapped\t%s\t%s\n", l, mapped[key])
		case strings.Contains(docs, key+")") || strings.Contains(docs, key+" ") || strings.Contains(docs, key+","), strings.Contains(docs, key+"."), strings.Contains(docs, key+"`"):
			fmt.Printf("doc-named\t%s\t\n", l)
		case seedMatch(xoset, pkg, kind, name) != "":
			fmt.Printf("seeded\t%s\t%s\n", l, seedMatch(xoset, pkg, kind, name))
		default:
			fmt.Printf("unmapped\t%s\t%s\n", l, kind)
		}
	}
	return nil
}

// rank counts, over the Go files under the corpus directories (non test,
// no testdata or vendor), the uses of each Go symbol of the report's
// unmapped rows: `alias.Name` selectors of an imported package for
// functions, types, consts and vars (exact, the import alias resolved), and
// `.Method` selectors in files that import the method's package for
// methods (an upper bound: the receiver's type is not resolved). It prints
// `COUNT<TAB>FILES<TAB>pkg kind name`, most used first.
func rank(repFile string, dirs []string, statuses map[string]bool) error {
	b, err := os.ReadFile(repFile)
	if err != nil {
		return err
	}
	type sym struct{ pkg, kind, name string }
	want := map[string]sym{}                 // "pkg name" -> symbol (non methods)
	methods := map[string]map[string][]sym{} // method name -> pkg -> symbols
	for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		f := strings.Split(l, "\t")
		if len(f) < 2 || !statuses[f[0]] {
			continue
		}
		g := strings.Fields(f[1])
		if len(g) != 3 {
			continue
		}
		sy := sym{g[0], g[1], g[2]}
		if sy.kind == "method" {
			_, m, _ := strings.Cut(sy.name, ".")
			if methods[m] == nil {
				methods[m] = map[string][]sym{}
			}
			methods[m][sy.pkg] = append(methods[m][sy.pkg], sy)
		} else {
			want[sy.pkg+" "+sy.name] = sy
		}
	}
	// a method name shared by several unmapped symbols (Name, String, Close)
	// cannot be told apart without the receiver's type: not counted
	for m, ps := range methods {
		n := 0
		for _, syms := range ps {
			n += len(syms)
		}
		if n > 1 {
			delete(methods, m)
		}
	}
	count := map[string]int{}
	files := map[string]int{}
	var nfiles int
	for _, root := range dirs {
		filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			n := d.Name()
			if d.IsDir() {
				if n == "testdata" || n == "vendor" || n == "_test" || strings.HasPrefix(n, ".") && n != "." {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
				return nil
			}
			if ok, err := ctxt.MatchFile(filepath.Dir(path), n); err != nil || !ok {
				return nil
			}
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return nil
			}
			nfiles++
			alias := map[string]string{}
			imported := map[string]bool{}
			for _, im := range f.Imports {
				p := strings.Trim(im.Path.Value, "\"")
				imported[p] = true
				a := p[strings.LastIndex(p, "/")+1:]
				if a == "v2" || a == "v3" || a == "v5" { // .../v2 packages are named by the parent
					q := p[:strings.LastIndex(p, "/")]
					a = q[strings.LastIndex(q, "/")+1:]
				}
				if im.Name != nil {
					a = im.Name.Name
				}
				alias[a] = p
			}
			seen := map[string]bool{}
			ast.Inspect(f, func(x ast.Node) bool {
				se, ok := x.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if id, ok := se.X.(*ast.Ident); ok && id.Obj == nil {
					if p, ok := alias[id.Name]; ok {
						k := p + " " + se.Sel.Name
						if _, w := want[k]; w {
							count[k]++
							if !seen[k] {
								seen[k] = true
								files[k]++
							}
						}
						return true
					}
				}
				if ms, ok := methods[se.Sel.Name]; ok {
					for p, syms := range ms {
						if !imported[p] {
							continue
						}
						for _, sy := range syms {
							k := sy.pkg + " " + sy.name
							count[k]++
							if !seen[k] {
								seen[k] = true
								files[k]++
							}
						}
					}
				}
				return true
			})
			return nil
		})
	}
	type row struct {
		k     string
		n, nf int
	}
	var rows []row
	for k, n := range count {
		rows = append(rows, row{k, n, files[k]})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].n != rows[j].n {
			return rows[i].n > rows[j].n
		}
		return rows[i].k < rows[j].k
	})
	fmt.Fprintf(os.Stderr, "stdaudit: %d files parsed\n", nfiles)
	for _, r := range rows {
		k := r.k
		// kind from the symbol tables
		pkg, name, _ := strings.Cut(k, " ")
		kind := "?"
		if sy, ok := want[k]; ok {
			kind = sy.kind
		} else {
			kind = "method"
		}
		fmt.Printf("%d\t%d\t%s %s %s\n", r.n, r.nf, pkg, kind, name)
	}
	return nil
}

func main() {
	goSide := flag.Bool("go", false, "print the Go API of the packages")
	rep := flag.Bool("report", false, "join -go-file and -xo-file output with -map")
	goFile := flag.String("go-file", "", "the output of -go (with -report)")
	xoFile := flag.String("xo-file", "", "the output of -xo (with -report)")
	mapFile := flag.String("map", "", "the mapping file (with -report)")
	xoPath := flag.String("xo", "", "print Xo's surface using this xo binary")
	stdDir := flag.String("std", "std", "the std directory (with -xo)")
	md := flag.String("md", "knowledgebase/infrastructure/std-coverage.md", "the coverage table")
	rankF := flag.String("rank", "", "rank the unmapped rows of this -report output by use in the corpus directories (arguments)")
	flag.Parse()
	if *rankF != "" {
		if err := rank(*rankF, flag.Args(), map[string]bool{"unmapped": true, "seeded": false}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if *rep {
		if err := report(*goFile, *xoFile, *mapFile, *md); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if *xoPath != "" {
		if err := xoSide(*xoPath, *stdDir); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if !*goSide {
		fmt.Fprintln(os.Stderr, "usage: stdaudit -go [-md FILE] [PKG...]")
		os.Exit(2)
	}
	pkgs := flag.Args()
	if len(pkgs) == 0 {
		var err error
		if pkgs, err = packages(*md); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	for _, p := range pkgs {
		syms, err := api(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "stdaudit: %s: %v\n", p, err)
			continue
		}
		for _, s := range syms {
			fmt.Println(s)
		}
	}
}
