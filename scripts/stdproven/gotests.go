package main

// gotests (task #357, the std semantics class guard): the cases of Go's own
// _test.go tables for the packages a module ports, each run through Go and
// through the module, every result compared, error text included.
//
//	go run ./scripts/stdproven gotests [-backends B,...] [-host LABEL] [-check] MODULE
//
// std/evidence/MODULE/gotests/ holds three files:
//
//   - tables.txt: one table a line, `PKG FILE VAR FIELDS KIND`: the Go
//     package (net/url), its test file (url_test.go), the table variable
//     (a package level var, or FUNC.NAME for a `NAME := []T{...}` inside
//     func FUNC), the input fields (comma separated: a field name, or a
//     position for an unkeyed element; a field written NAME:const also
//     takes an exported constant of the package, or a | of them, as its
//     name, "Debug|AlignRight", for the adapters to map; a field written
//     NAME=DEFAULT reads as DEFAULT where a keyed element leaves NAME out,
//     instead of skipping the element; FIELDS "." reads each element of a
//     table of values, []int64{...}, as itself), and the kind the
//     adapters know it by. Blank lines and # comments are skipped.
//   - go/main.go: the Go adapter, `main CASES`, printing a result line per
//     case.
//   - main.xo: the Xo adapter, the same protocol.
//
// The runner reads each table from the pinned Go release's source (go env
// GOROOT) with go/ast and takes its input fields' literals as text (a
// string literal or a + of them, an integer, an integral float such as
// 1e9, a rune literal as its code point, a slice of integer or rune
// literals as its values joined with commas (a FIELDS entry NAME:json reads
// a slice of strings or of structs of them as JSON), true and false, and time's zone
// names Local and UTC); an element whose input is
// anything else is counted as skipped, as is one that is not UTF-8 (an Xo
// Str is). A case is a line `KIND<TAB>B64<TAB>B64...` (the
// inputs in base64, so any byte passes). Each adapter prints one line a
// case, the base64 of its result text (the adapters agree on that text: a
// result's fields, or "err " and the error's text). Go's answer is Go's
// own: the adapter calls the Go function, so the tables' expected values,
// which reference package internals, are not needed. Every Xo result must
// equal Go's.
//
// A pass writes std/evidence/MODULE/gotests.md. -check prints the result
// and writes nothing (for a fix's test loop).

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// goTable is a line of tables.txt.
type goTable struct {
	pkg, file, name, kind string
	fields                []string
	// dir is the package's source when it is not in GOROOT (a
	// golang.org/x package the Go adapter's go.mod requires)
	dir string
}

// goCase is one table element's inputs.
type goCase struct {
	kind   string
	table  string
	index  int
	inputs []string
}

func readGoTables(path string) ([]goTable, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ts []goTable
	for n, line := range strings.Split(string(text), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) != 5 {
			return nil, fmt.Errorf("%s:%d: want PKG FILE VAR FIELDS KIND, got %q", path, n+1, line)
		}
		ts = append(ts, goTable{pkg: f[0], file: f[1], name: f[2], fields: strings.Split(f[3], ","), kind: f[4]})
	}
	if len(ts) == 0 {
		return nil, fmt.Errorf("%s: no tables", path)
	}
	return ts, nil
}

// fieldName is a FIELDS entry's name, without =DEFAULT, :bytes, :json, :const
// or @PATTERN.
func fieldName(f string) string {
	f, _, _ = strings.Cut(f, "=")
	f = strings.TrimSuffix(f, ":bytes")
	f = strings.TrimSuffix(f, ":json")
	f = strings.TrimSuffix(f, ":const")
	f, _, _ = strings.Cut(f, "@")
	return f
}

// goTableCases reads table t from the Go source under goroot: its
// elements' inputs, and how many elements had an input that is no literal.
func goTableCases(goroot string, t goTable) ([]goCase, int, error) {
	path := filepath.Join(goroot, "src", filepath.FromSlash(t.pkg), t.file)
	if t.dir != "" {
		path = filepath.Join(t.dir, t.file)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, 0, err
	}
	lit, err := findTable(f, t.name)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: %v", path, err)
	}
	order := structOrder(f, lit.Type)
	var cases []goCase
	skipped := 0
	for i, el := range lit.Elts {
		c, ok := el.(*ast.CompositeLit)
		if !ok {
			if u, isU := el.(*ast.UnaryExpr); isU {
				c, ok = u.X.(*ast.CompositeLit)
			}
		}
		if !ok && !(len(t.fields) == 1 && fieldName(t.fields[0]) == ".") {
			skipped++
			continue
		}
		in := make([]string, 0, len(t.fields))
		for _, field := range t.fields {
			field, def, hasDef := strings.Cut(field, "=")
			name, isBytes := strings.CutSuffix(field, ":bytes")
			name, isConst := strings.CutSuffix(name, ":const")
			name, isJSON := strings.CutSuffix(name, ":json")
			// NAME@PATTERN: the input is the bytes of the file at
			// PATTERN (beside the _test.go file) with {} the value
			name, pattern, isFile := strings.Cut(name, "@")
			var e ast.Expr
			if name == "." {
				e = el
			} else {
				e = fieldExpr(c, name, order)
			}
			// a default fills a field left out or nil (zlibTests' dict)
			if hasDef && (e == nil || isNil(e)) {
				in = append(in, def)
				continue
			}
			s, ok := stringLit(e)
			if isJSON {
				s, ok = jsonLit(f, e)
			}
			if !ok && isConst {
				s, ok = constNames(e)
			}
			// a field marked :bytes may be any bytes (the adapter reads
			// them as Bytes); others must be UTF-8, which Str holds
			if ok && isFile {
				b, err := os.ReadFile(filepath.Join(filepath.Dir(path), filepath.FromSlash(strings.ReplaceAll(pattern, "{}", s))))
				s, ok, isBytes = string(b), err == nil, true
			}
			if !ok || (!isBytes && !utf8.ValidString(s)) {
				break
			}
			in = append(in, s)
		}
		if len(in) != len(t.fields) {
			skipped++
			continue
		}
		cases = append(cases, goCase{kind: t.kind, table: t.name, index: i, inputs: in})
	}
	if len(cases) == 0 {
		return nil, 0, fmt.Errorf("%s: table %s gave no cases (%d skipped)", path, t.name, skipped)
	}
	return cases, skipped, nil
}

// findTable is the composite literal of a package level var NAME, or of
// `NAME := ...` inside func FUNC for FUNC.NAME.
func findTable(f *ast.File, name string) (*ast.CompositeLit, error) {
	fn, local, inFunc := strings.Cut(name, ".")
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.GenDecl:
			if inFunc || d.Tok != token.VAR {
				continue
			}
			for _, s := range d.Specs {
				vs := s.(*ast.ValueSpec)
				for i, id := range vs.Names {
					if id.Name == name && i < len(vs.Values) {
						if c, ok := vs.Values[i].(*ast.CompositeLit); ok {
							return c, nil
						}
					}
				}
			}
		case *ast.FuncDecl:
			if !inFunc || d.Name.Name != fn || d.Body == nil {
				continue
			}
			var found *ast.CompositeLit
			if nth, ok := rangeIndex(local); ok {
				// FUNC.range (FUNC.rangeN): the Nth literal a for
				// statement of the function ranges over, in source order.
				ast.Inspect(d.Body, func(n ast.Node) bool {
					r, ok := n.(*ast.RangeStmt)
					if !ok || found != nil {
						return found == nil
					}
					if c, ok := r.X.(*ast.CompositeLit); ok {
						if nth--; nth == 0 {
							found = c
						}
					}
					return true
				})
				if found != nil {
					return found, nil
				}
				continue
			}
			ast.Inspect(d.Body, func(n ast.Node) bool {
				a, ok := n.(*ast.AssignStmt)
				if !ok || found != nil {
					return found == nil
				}
				for i, l := range a.Lhs {
					if id, ok := l.(*ast.Ident); ok && id.Name == local && i < len(a.Rhs) {
						if c, ok := a.Rhs[i].(*ast.CompositeLit); ok {
							found = c
						}
					}
				}
				return true
			})
			if found != nil {
				return found, nil
			}
		}
	}
	return nil, fmt.Errorf("no table %s", name)
}

// rangeIndex reads a table name's local part `range` (1) or `rangeN`
// (N, from 1).
func rangeIndex(local string) (int, bool) {
	rest, ok := strings.CutPrefix(local, "range")
	if !ok {
		return 0, false
	}
	if rest == "" {
		return 1, true
	}
	n, err := strconv.Atoi(rest)
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

// structOrder is the field names, in order, of the element type of a
// slice literal's type: an inline struct, or a named struct type of the
// file.
func structOrder(f *ast.File, t ast.Expr) []string {
	at, ok := t.(*ast.ArrayType)
	if !ok {
		return nil
	}
	el := at.Elt
	if st, ok := el.(*ast.StarExpr); ok {
		el = st.X
	}
	var s *ast.StructType
	switch e := el.(type) {
	case *ast.StructType:
		s = e
	case *ast.Ident:
		ast.Inspect(f, func(n ast.Node) bool {
			if ts, ok := n.(*ast.TypeSpec); ok && ts.Name.Name == e.Name {
				s, _ = ts.Type.(*ast.StructType)
			}
			return s == nil
		})
	}
	if s == nil {
		return nil
	}
	var names []string
	for _, fl := range s.Fields.List {
		if len(fl.Names) == 0 {
			names = append(names, "")
		}
		for _, n := range fl.Names {
			names = append(names, n.Name)
		}
	}
	return names
}

// fieldExpr is the value of field (a name or a position) in element c.
func fieldExpr(c *ast.CompositeLit, field string, order []string) ast.Expr {
	pos, err := strconv.Atoi(field)
	if err != nil {
		pos = -1
		for i, n := range order {
			if n == field {
				pos = i
			}
		}
	}
	for i, e := range c.Elts {
		if kv, ok := e.(*ast.KeyValueExpr); ok {
			if id, ok := kv.Key.(*ast.Ident); ok && id.Name == field {
				return kv.Value
			}
			continue
		}
		if i == pos {
			return e
		}
	}
	return nil
}

// stringLit is the value of a string literal or a + of them.
// isNil reports whether e is the identifier nil.
func isNil(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "nil"
}

func stringLit(e ast.Expr) (string, bool) {
	if v, ok := intConst(e); ok {
		return v.ExactString(), true
	}
	switch e := e.(type) {
	case *ast.BasicLit:
		switch e.Kind {
		case token.INT:
			n, err := strconv.ParseInt(strings.ReplaceAll(e.Value, "_", ""), 0, 64)
			return strconv.FormatInt(n, 10), err == nil
		case token.FLOAT:
			// an integer written as a float (1e9)
			f, err := strconv.ParseFloat(e.Value, 64)
			if err != nil || f != float64(int64(f)) {
				return "", false
			}
			return strconv.FormatInt(int64(f), 10), true
		case token.STRING:
			s, err := strconv.Unquote(e.Value)
			return s, err == nil
		case token.CHAR:
			// a rune literal as its code point ('.' is 46)
			r, _, _, err := strconv.UnquoteChar(e.Value[1:len(e.Value)-1], '\'')
			return strconv.Itoa(int(r)), err == nil
		}
		return "", false
	case *ast.Ident:
		// time's zones: the adapters read Local as Go's tests set it
		// (America/Los_Angeles) and UTC
		if e.Name == "Local" || e.Name == "UTC" || e.Name == "true" || e.Name == "false" {
			return e.Name, true
		}
		return "", false
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return "", false
		}
		a, ok1 := stringLit(e.X)
		b, ok2 := stringLit(e.Y)
		return a + b, ok1 && ok2
	case *ast.ParenExpr:
		return stringLit(e.X)
	case *ast.CallExpr:
		// conversions of a literal ([]byte("..."), string("...")) are the
		// literal; strings.Repeat of a literal and a constant count is its
		// text (up to 1 MB)
		if len(e.Args) == 1 && !e.Ellipsis.IsValid() {
			if id, ok := e.Fun.(*ast.Ident); ok && id.Name == "string" {
				return stringLit(e.Args[0])
			}
			if t, ok := e.Fun.(*ast.ArrayType); ok && t.Len == nil {
				if id, ok := t.Elt.(*ast.Ident); ok && id.Name == "byte" {
					return stringLit(e.Args[0])
				}
			}
		}
		// a must function of one literal (mustIP("1.2.3.4"),
		// netip.MustParseAddr("::1")) is the literal: the adapters parse it
		if len(e.Args) == 1 && !e.Ellipsis.IsValid() && mustCall(e.Fun) {
			return stringLit(e.Args[0])
		}
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok && len(e.Args) == 2 {
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "strings" && sel.Sel.Name == "Repeat" {
				s, ok1 := stringLit(e.Args[0])
				n, ok2 := intConst(e.Args[1])
				if !ok1 || !ok2 || intLit(e.Args[0]) {
					return "", false
				}
				c, exact := constant.Int64Val(n)
				if !exact || c < 0 || int64(len(s))*c > 1<<20 {
					return "", false
				}
				return strings.Repeat(s, int(c)), true
			}
		}
		return "", false
	case *ast.CompositeLit:
		// a slice of integer or rune literals ([]rune{'a', 0x10000},
		// []uint16{1, -1}) is its values joined with commas
		if t, ok := e.Type.(*ast.ArrayType); !ok || t.Len != nil {
			return "", false
		}
		vals := make([]string, len(e.Elts))
		for i, el := range e.Elts {
			if !intLit(el) {
				return "", false
			}
			v, ok := stringLit(el)
			if !ok {
				return "", false
			}
			vals[i] = v
		}
		return strings.Join(vals, ","), true
	}
	return "", false
}

// jsonLit is a :json field's value as JSON: nil is null, a string literal
// a string, a slice literal an array of its elements, and a struct
// literal (an element of a slice of structs) an array of its fields in
// the struct's order, a field left out "". Anything else is no literal.
func jsonLit(f *ast.File, e ast.Expr) (string, bool) {
	v, ok := jsonVal(f, e, nil)
	if !ok {
		return "", false
	}
	b, err := json.Marshal(v)
	return string(b), err == nil
}

// jsonVal is e's JSON value; outer is the slice type e is an element of.
func jsonVal(f *ast.File, e ast.Expr, outer ast.Expr) (any, bool) {
	if isNil(e) {
		return nil, true
	}
	c, ok := e.(*ast.CompositeLit)
	if !ok {
		s, ok := stringLit(e)
		return s, ok
	}
	if at, ok := c.Type.(*ast.ArrayType); ok {
		out := []any{}
		for _, el := range c.Elts {
			v, ok := jsonVal(f, el, at)
			if !ok {
				return nil, false
			}
			out = append(out, v)
		}
		return out, true
	}
	names := structOrder(f, outer)
	if c.Type != nil || len(names) == 0 {
		return nil, false
	}
	out := make([]any, len(names))
	for i, n := range names {
		fe := fieldExpr(c, n, names)
		if fe == nil {
			out[i] = ""
			continue
		}
		v, ok := jsonVal(f, fe, nil)
		if !ok {
			return nil, false
		}
		out[i] = v
	}
	return out, true
}

// intLit reports whether e is an integer or rune literal, negated or not.
func intLit(e ast.Expr) bool {
	_, ok := intConst(e)
	return ok
}

// intConst folds an integer constant expression of literals: integer and
// rune literals under unary -, + and ^, the binary integer operators, and
// parentheses (-1<<63 + 1 is -9223372036854775807).
func intConst(e ast.Expr) (constant.Value, bool) {
	switch e := e.(type) {
	case *ast.BasicLit:
		if e.Kind != token.INT && e.Kind != token.CHAR {
			return nil, false
		}
		v := constant.MakeFromLiteral(e.Value, e.Kind, 0)
		if e.Kind == token.CHAR {
			v = constant.ToInt(v)
		}
		return v, v.Kind() == constant.Int
	case *ast.ParenExpr:
		return intConst(e.X)
	case *ast.UnaryExpr:
		x, ok := intConst(e.X)
		if !ok || (e.Op != token.SUB && e.Op != token.ADD && e.Op != token.XOR) {
			return nil, false
		}
		return constant.UnaryOp(e.Op, x, 0), true
	case *ast.BinaryExpr:
		x, ok1 := intConst(e.X)
		y, ok2 := intConst(e.Y)
		if !ok1 || !ok2 {
			return nil, false
		}
		switch e.Op {
		case token.SHL, token.SHR:
			n, ok := constant.Uint64Val(y)
			if !ok || n > 1024 {
				return nil, false
			}
			return constant.Shift(x, e.Op, uint(n)), true
		case token.QUO, token.REM:
			if constant.Sign(y) == 0 {
				return nil, false
			}
			op := e.Op
			if op == token.QUO {
				op = token.QUO_ASSIGN // integer division
			}
			return constant.BinaryOp(x, op, y), true
		case token.ADD, token.SUB, token.MUL, token.AND, token.OR, token.XOR, token.AND_NOT:
			return constant.BinaryOp(x, e.Op, y), true
		}
	}
	return nil, false
}

// constNames is an exported constant of the table's package, or a | of
// them, as its names (`Debug|AlignRight`), for a NAME:const field.
func constNames(e ast.Expr) (string, bool) {
	switch e := e.(type) {
	case *ast.Ident:
		return e.Name, ast.IsExported(e.Name)
	case *ast.BinaryExpr:
		if e.Op != token.OR {
			return "", false
		}
		a, ok1 := constNames(e.X)
		b, ok2 := constNames(e.Y)
		return a + "|" + b, ok1 && ok2
	case *ast.ParenExpr:
		return constNames(e.X)
	}
	return "", false
}

// goCasesFile is the cases in the adapters' protocol.
func goCasesFile(cases []goCase) []byte {
	var b bytes.Buffer
	for _, c := range cases {
		b.WriteString(c.kind)
		for _, in := range c.inputs {
			b.WriteByte('\t')
			b.WriteString(base64.StdEncoding.EncodeToString([]byte(in)))
		}
		b.WriteByte('\n')
	}
	return b.Bytes()
}

// adapterLines decodes an adapter's output: one result a case.
func adapterLines(who string, out []byte, n int) ([]string, error) {
	// only the last line's newline: a result of "" is an empty line
	text := strings.TrimSuffix(string(out), "\n")
	var lines []string
	if n > 0 || text != "" {
		lines = strings.Split(text, "\n")
	}
	if len(lines) != n {
		tail := text
		if len(tail) > 4000 {
			tail = tail[len(tail)-4000:]
		}
		return nil, fmt.Errorf("%s printed %d lines for %d cases:\n%s", who, len(lines), n, tail)
	}
	for i, l := range lines {
		d, err := base64.StdEncoding.DecodeString(l)
		if err != nil {
			return nil, fmt.Errorf("%s line %d is no base64: %q", who, i+1, l)
		}
		lines[i] = string(d)
	}
	return lines, nil
}

func goTests(args []string) error {
	fs := flag.NewFlagSet("gotests", flag.ExitOnError)
	backends := fs.String("backends", "arm64,llvm", "the backends the Xo adapter runs on")
	hostLabel := fs.String("host", "", "the host line of the evidence")
	check := fs.Bool("check", false, "compare and print, write no evidence")
	fs.Parse(args)
	if fs.NArg() != 1 {
		usage()
	}
	module := fs.Arg(0)
	dir := filepath.Join("std", "evidence", module, "gotests")
	tables, err := readGoTables(filepath.Join(dir, "tables.txt"))
	if err != nil {
		return err
	}
	gr, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		return fmt.Errorf("go env GOROOT: %v", err)
	}
	goroot := strings.TrimSpace(string(gr))
	gv, err := exec.Command("go", "env", "GOVERSION").Output()
	if err != nil {
		return fmt.Errorf("go env GOVERSION: %v", err)
	}
	goVersion := strings.TrimSpace(string(gv))
	// a Go adapter with its own go.mod (go/go.mod) requires the module a
	// port follows (golang.org/x/net for htmltok): a table's package not
	// in GOROOT is read from that module, at the version go.mod pins
	modDir := filepath.Join(dir, "go")
	_, err = os.Stat(filepath.Join(modDir, "go.mod"))
	ownMod := err == nil
	seenMod := map[string]bool{}
	for i, t := range tables {
		if _, err := os.Stat(filepath.Join(goroot, "src", filepath.FromSlash(t.pkg))); err == nil || !ownMod {
			continue
		}
		out, err := exec.Command("go", "list", "-C", modDir, "-f", "{{.Dir}}|{{.Module.Path}} {{.Module.Version}}", t.pkg).Output()
		if err != nil {
			return fmt.Errorf("go list %s in %s: %v", t.pkg, modDir, err)
		}
		d, m, _ := strings.Cut(strings.TrimSpace(string(out)), "|")
		tables[i].dir = d
		if !seenMod[m] {
			seenMod[m] = true
			goVersion += ", " + m
		}
	}
	var cases []goCase
	var tableLines []string
	skipped := 0
	for _, t := range tables {
		cs, sk, err := goTableCases(goroot, t)
		if err != nil {
			return err
		}
		cases = append(cases, cs...)
		skipped += sk
		tableLines = append(tableLines, fmt.Sprintf("- %s %s %s (%s): %d cases, %d skipped", t.pkg, t.file, t.name, t.kind, len(cs), sk))
	}
	var src, commit string
	if !*check {
		if src, _, err = sourceHash(module); err != nil {
			return err
		}
		if commit, err = gitOut("rev-parse", "--short=9", "HEAD"); err != nil {
			return err
		}
		if dirty, _ := gitOut("status", "--porcelain", "--", "std", ":(exclude)std/evidence", "compiler"); dirty != "" {
			return fmt.Errorf("std or compiler has uncommitted changes: evidence names a commit:\n%s", dirty)
		}
	}
	tmp, err := os.MkdirTemp("", "stdproven-gotests-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	casesPath := filepath.Join(tmp, "cases.txt")
	if err := os.WriteFile(casesPath, goCasesFile(cases), 0o644); err != nil {
		return err
	}
	gbin := filepath.Join(tmp, "go-adapter")
	build := exec.Command("go", "build", "-o", gbin, "./"+filepath.ToSlash(modDir))
	if ownMod {
		build = exec.Command("go", "build", "-C", modDir, "-o", gbin, ".")
	}
	if out, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("go build %s/go: %v\n%s", dir, err, out)
	}
	gout, err := exec.Command(gbin, casesPath).Output()
	if err != nil {
		return fmt.Errorf("the Go adapter: %v", err)
	}
	want, err := adapterLines("the Go adapter", gout, len(cases))
	if err != nil {
		return err
	}
	var runLines []string
	bad := 0
	for _, b := range strings.Split(*backends, ",") {
		xbin := filepath.Join(tmp, "xo-adapter-"+b)
		if out, err := exec.Command(filepath.Join("bin", "xo"), "build", "--backend="+b, "-o", xbin, dir).CombinedOutput(); err != nil {
			return fmt.Errorf("xo build --backend=%s %s: %v\n%s", b, dir, err, out)
		}
		t0 := time.Now()
		var stderr bytes.Buffer
		cmd := exec.Command(xbin, casesPath)
		cmd.Stderr = &stderr
		xout, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("the Xo adapter on %s: %v\n%s", b, err, stderr.String())
		}
		got, err := adapterLines("the Xo adapter on "+b, xout, len(cases))
		if err != nil {
			return err
		}
		miss := 0
		for i := range cases {
			if got[i] == want[i] {
				continue
			}
			miss++
			if miss <= 20 {
				c := cases[i]
				fmt.Printf("MISMATCH %s %s[%d] %q:\n  go: %q\n  xo: %q\n", b, c.table, c.index, c.inputs, want[i], got[i])
			}
		}
		bad += miss
		line := fmt.Sprintf("- %s: %d cases, %d mismatches, %s", b, len(cases), miss, time.Since(t0).Round(time.Millisecond))
		fmt.Println(line)
		runLines = append(runLines, line)
	}
	if bad > 0 {
		return fmt.Errorf("%d mismatches with Go (%s): each is a fix with a test, then a new run", bad, goVersion)
	}
	if *check {
		return nil
	}
	host := *hostLabel
	if host == "" {
		host = runtime.GOOS + "/" + runtime.GOARCH
	}
	var b strings.Builder
	fmt.Fprintf(&b, "kind: gotests\nmodule: %s\nresult: pass\ncommit: %s\nsource: %s\ndate: %s\nhost: %s\n", module, commit, src, time.Now().UTC().Format("2006-01-02"), host)
	fmt.Fprintf(&b, "command: go run ./scripts/stdproven gotests -backends %s -host %q %s\n", *backends, host, module)
	fmt.Fprintf(&b, "measure: %d cases from %s of Go's tests (%s), %d backends (%s), 0 mismatches with Go, %s skipped (inputs not literals)\n\n", len(cases), plural(len(tables), "table"), goVersion, len(strings.Split(*backends, ",")), *backends, plural(skipped, "element"))
	fmt.Fprintf(&b, "Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (%s/go) and the module (%s/main.xo), every result equal, error text included.\n\n", dir, dir)
	b.WriteString(strings.Join(tableLines, "\n") + "\n\n")
	b.WriteString(strings.Join(runLines, "\n") + "\n")
	return os.WriteFile(filepath.Join("std", "evidence", module, "gotests.md"), []byte(b.String()), 0o644)
}

// plural is "1 table" or "N tables".
func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// mustCall reports whether fun names a function whose name starts with
// must or Must (a test's parse-or-panic helper).
func mustCall(fun ast.Expr) bool {
	name := ""
	switch f := fun.(type) {
	case *ast.Ident:
		name = f.Name
	case *ast.SelectorExpr:
		name = f.Sel.Name
	}
	return strings.HasPrefix(name, "must") || strings.HasPrefix(name, "Must")
}
