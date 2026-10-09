// Package check resolves names and type checks xo modules (core 2 to 9).
//
// The checker loads the modules named on the command line, the user modules
// they import (through xo.mod), and the std declaration files (std/*.xo),
// then checks every function, method, test, constant, contract, and default
// value. It reports core 10.2 diagnostics with stable codes and never
// cascades: a bad expression gets the `<error>` type, which is compatible
// with everything and suppresses dependent diagnostics.
//
// Phases: load and parse (load.go), declare and resolve signatures
// (decl.go), check bodies (expr.go, call.go, stmt.go, pattern.go), then
// solve effects (effects.go), and run the flow checks for moves and lost
// updates (flow.go).
package check

import (
	"fmt"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/token"
)

// Result is the outcome of a check run.
type Result struct {
	Modules     []*Module
	Diagnostics []diag.Diagnostic

	c *checker
}

type checker struct {
	diags   diag.List
	seen    map[string]bool // de-duplication of diagnostics
	modules map[string]*Module
	order   []*Module // dependency order
	prelude *Module
	roots   []modRoot
	// goFailed holds `go:<path>` keys of Go packages that could not be
	// bound (XO1101 reported once per run).
	goFailed map[string]bool
	// overlay maps absolute .xo paths to contents that replace the files
	// on disk (CheckOverlay).
	overlay map[string][]byte
	// modErr is the first malformed xo.mod line found (an unknown edition).
	modErr error
	// deps are the resolved requirements of the main modules (deps.go).
	deps depState
	// ceil caches the effect ceilings of dependencies (ceiling.go).
	ceil ceilingState

	nextVar int
	trail   []func()

	inferredFns []*FuncObj
	inferred    map[*FuncObj]*flatEff
	obligations []effObligation

	// renames are XO0101 fixes of local names whose uses are added once
	// the bodies are checked (decision 0087).
	renames []pendingRename
	// safeNavs are `x?.f` optional chains reported as XO0205.
	safeNavs    map[*ast.TryExpr]bool
	safeNavDiag map[*ast.TryExpr]int // index of its XO0205 in c.diags.Items

	// per body information used by the flow checks
	identObj map[*ast.Ident]*VarObj
	exprType map[ast.Expr]Type
	// neverExpr marks expressions of type Never checked against an
	// expected type (exprType then holds the expected type).
	neverExpr map[ast.Expr]bool
	callInfo  map[*ast.CallExpr]*callInfo
	ctorCalls map[*ast.CallExpr]bool
	// rec holds resolutions recorded for code generation (info.go). They
	// never influence what the checker accepts.
	rec      *records
	bodies   []*bodyInfo
	unused   []*VarObj
	lostVars map[*VarObj]bool
	// splitWarned dedupes XO0804 across nested bodies.
	splitWarned map[*ast.CallExpr]bool
	// mapStore is the target of a plain `m[k] = v`, the only Map index
	// use that cannot fault (core 3.6).
	mapStore *ast.IndexExpr
	// typeArgVars are inference variables of explicit type parameters at
	// calls; unbound ones at the end are XO0402.
	typeArgVars []typeArgVar
	// neverResults are the result types of closures whose body never
	// completes (an infinite `while`, a `fault`): when nothing else
	// determines one, it is `Never` (core 2.1, 3.2), not XO0402.
	neverResults []Type

	// current position
	file *File
	mod  *Module
	ctx  *fnCtx
	sc   *scope
}

// callInfo records what a call resolved to, for the flow checks.
type callInfo struct {
	fn       *FuncObj // declared callee, if any
	recvExpr ast.Expr // receiver of a method call
	params   []*argParam
	sm       *substMap // type arguments of the callee
}

// argParam pairs an argument with whether its parameter is `var`.
type argParam struct {
	arg   ast.Expr
	isVar bool
}

// bodyInfo is one checked body (function, test, closure owner) for the flow
// checks.
type bodyInfo struct {
	body   *ast.Block
	params []*VarObj
}

func newChecker() *checker {
	return &checker{
		seen:        map[string]bool{},
		modules:     map[string]*Module{},
		identObj:    map[*ast.Ident]*VarObj{},
		safeNavs:    map[*ast.TryExpr]bool{},
		safeNavDiag: map[*ast.TryExpr]int{},
		exprType:    map[ast.Expr]Type{},
		neverExpr:   map[ast.Expr]bool{},
		callInfo:    map[*ast.CallExpr]*callInfo{},
		lostVars:    map[*VarObj]bool{},
		splitWarned: map[*ast.CallExpr]bool{},
		rec:         newRecords(),
	}
}

// Check loads and checks the modules found under dirs (every directory that
// contains .xo files is a module).
func Check(dirs []string) (*Result, error) {
	return CheckOverlay(dirs, nil)
}

// CheckOverlay is Check with some source files replaced: overlay maps an
// absolute .xo path to the contents to check instead of the file on disk.
// Diagnostics still name the files by their usual paths. `xo fix` uses it to
// check rewritten sources without writing them.
func CheckOverlay(dirs []string, overlay map[string][]byte) (*Result, error) {
	return CheckOpts(dirs, Options{Overlay: overlay})
}

// Options configure CheckOpts.
type Options struct {
	// Overlay maps absolute .xo paths to contents that replace the files
	// on disk (CheckOverlay).
	Overlay map[string][]byte
	// Native also loads the implementation modules of the native
	// backends (std/native/*.xo, decision 0074): for the prelude and for
	// every std module the program uses that has one. They are checked
	// like user modules and lowered by internal/ssa in place of the std
	// declarations.
	Native bool
}

// CheckOpts is Check with options.
func CheckOpts(dirs []string, opts Options) (*Result, error) {
	c := newChecker()
	c.overlay = opts.Overlay
	if err := c.loadPrelude(); err != nil {
		return nil, err
	}
	roots, err := c.loadDirs(dirs)
	if err != nil {
		return nil, err
	}
	if c.modErr != nil {
		return nil, c.modErr
	}
	if opts.Native {
		if err := c.loadNative(); err != nil {
			return nil, err
		}
	}
	c.run()
	res := &Result{Modules: roots, c: c}
	res.Diagnostics = c.diags.Items
	diag.Sort(res.Diagnostics)
	return res, nil
}

func (c *checker) run() {
	for _, m := range c.order {
		c.declare(m)
	}
	for _, m := range c.order {
		c.resolveTypes(m)
	}
	for _, m := range c.order {
		c.resolveSigs(m)
	}
	for _, m := range c.order {
		if !m.IsStd {
			c.checkModule(m)
			c.runEditionRules(m)
		}
	}
	c.solveEffects()
	for _, b := range c.bodies {
		c.flowBody(b)
	}
	c.reportUnused()
	c.reportAmbiguous()
	c.finishRenames()
	for _, m := range c.order {
		if m.IsStd {
			continue
		}
		for _, f := range m.Files {
			c.checkUnusedImports(f)
		}
	}
}

// errorf reports an error, dropping exact duplicates. It returns nil for a
// duplicate so callers must check before adding notes or fixes.
func (c *checker) errorf(code diag.Code, sp token.Span, format string, args ...any) *diag.Diagnostic {
	return c.report(diag.Error, code, sp, format, args...)
}

func (c *checker) warnf(code diag.Code, sp token.Span, format string, args ...any) *diag.Diagnostic {
	return c.report(diag.Warning, code, sp, format, args...)
}

func (c *checker) report(sev diag.Severity, code diag.Code, sp token.Span, format string, args ...any) *diag.Diagnostic {
	msg := fmt.Sprintf(format, args...)
	key := fmt.Sprintf("%s|%s|%d:%d|%s", code, sp.File, sp.Start.Line, sp.Start.Col, msg)
	if c.seen[key] {
		return nil
	}
	c.seen[key] = true
	d := c.diags.Errorf(code, sp, "%s", msg)
	d.Severity = sev
	return d
}

func note(d *diag.Diagnostic, sp token.Span, format string, args ...any) {
	if d == nil {
		return
	}
	n := diag.NoteItem{Message: fmt.Sprintf(format, args...)}
	if sp.File != "" {
		s := diag.FromSpan(sp)
		s.EndLine, s.EndCol = 0, 0
		n.Span = &s
	}
	d.Notes = append(d.Notes, n)
}

func fix(d *diag.Diagnostic, title string, edits ...diag.Edit) {
	if d == nil {
		return
	}
	d.Fixes = append(d.Fixes, diag.Fix{Title: title, Edits: edits})
}

func replaceEdit(sp token.Span, text string) diag.Edit {
	return diag.Edit{Span: diag.FromSpan(sp), Replace: text}
}

func insertEdit(file string, p token.Pos, text string) diag.Edit {
	return diag.Edit{Span: diag.PointSpan(file, p), Replace: text}
}

// ---------------------------------------------------------------------------
// Prelude lookups

func (c *checker) preludeType(name string) *TypeObj {
	if o, ok := c.prelude.Scope[name].(*TypeObj); ok {
		return o
	}
	panic("prelude type missing: " + name)
}

func (c *checker) named(name string, args ...Type) *Named {
	return &Named{Obj: c.preludeType(name), Args: args}
}

func (c *checker) optionOf(t Type) Type { return c.named("Option", t) }

func (c *checker) resultOf(t, e Type) Type { return c.named("Result", t, e) }

// callResult is the type a caller sees for a function returning t ! e.
func (c *checker) callResult(t, e Type) Type {
	if e == nil || c.isNever(e) {
		return t
	}
	return c.resultOf(t, e)
}

// isPrelude reports whether t is the prelude type name.
func (c *checker) isPrelude(t Type, name string) bool {
	n, ok := resolve(t).(*Named)
	return ok && n.Obj.Module == c.prelude && n.Obj.Name == name
}

func (c *checker) isNever(t Type) bool { return c.isPrelude(t, "Never") }

func isNever(t Type) bool {
	n, ok := resolve(t).(*Named)
	return ok && n.Obj.Module != nil && n.Obj.Module.IsPrelude && n.Obj.Name == "Never"
}

// optionElem returns T for T? and nil otherwise.
func (c *checker) optionElem(t Type) Type {
	if n, ok := resolve(t).(*Named); ok && n.Obj.Module == c.prelude && n.Obj.Name == "Option" {
		return n.Args[0]
	}
	return nil
}

// resultParts returns T and E for Result[T, E].
func (c *checker) resultParts(t Type) (Type, Type, bool) {
	if n, ok := resolve(t).(*Named); ok && n.Obj.Module == c.prelude && n.Obj.Name == "Result" {
		return n.Args[0], n.Args[1], true
	}
	return nil, nil, false
}

var intTypes = map[string]bool{"Int": true, "Int8": true, "Int16": true, "Int32": true, "Int64": true, "UInt8": true, "UInt16": true, "UInt32": true, "UInt64": true}
var floatTypes = map[string]bool{"Float": true, "Float32": true}

func (c *checker) isInt(t Type) bool {
	n, ok := resolve(t).(*Named)
	return ok && n.Obj.Module == c.prelude && intTypes[n.Obj.Name]
}

func (c *checker) isFloat(t Type) bool {
	n, ok := resolve(t).(*Named)
	return ok && n.Obj.Module == c.prelude && floatTypes[n.Obj.Name]
}

func (c *checker) isNumeric(t Type) bool { return c.isInt(t) || c.isFloat(t) }

func (c *checker) isInterface(t Type) bool {
	n, ok := resolve(t).(*Named)
	return ok && n.Obj.Kind == KInterface
}

func (c *checker) newVar() *Var {
	c.nextVar++
	return &Var{id: c.nextVar}
}

// ---------------------------------------------------------------------------
// Naming helpers (core 1.5)

func isUpperCamel(s string) bool {
	if s == "" || s[0] < 'A' || s[0] > 'Z' {
		return false
	}
	for i := 1; i < len(s); i++ {
		ch := s[i]
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9') {
			return false
		}
	}
	return true
}

func isSnake(s string) bool {
	t := strings.TrimLeft(s, "_")
	if t == "" {
		return true
	}
	if t[0] < 'a' || t[0] > 'z' {
		return false
	}
	for i := 1; i < len(t); i++ {
		ch := t[i]
		if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '_') {
			return false
		}
	}
	return true
}

func isUpperSnake(s string) bool {
	if s == "" || s[0] < 'A' || s[0] > 'Z' {
		return false
	}
	for i := 1; i < len(s); i++ {
		ch := s[i]
		if !(ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_') {
			return false
		}
	}
	return true
}

// words splits an identifier in any convention into lower case words.
func words(s string) []string {
	var out []string
	var cur []rune
	rs := []rune(s)
	flush := func() {
		if len(cur) > 0 {
			out = append(out, strings.ToLower(string(cur)))
			cur = nil
		}
	}
	for i, r := range rs {
		switch {
		case r == '_' || r == '-':
			flush()
		case r >= 'A' && r <= 'Z':
			prevLower := i > 0 && (rs[i-1] >= 'a' && rs[i-1] <= 'z' || rs[i-1] >= '0' && rs[i-1] <= '9')
			nextLower := i+1 < len(rs) && rs[i+1] >= 'a' && rs[i+1] <= 'z'
			prevUpper := i > 0 && rs[i-1] >= 'A' && rs[i-1] <= 'Z'
			if prevLower || (prevUpper && nextLower) {
				flush()
			}
			cur = append(cur, r)
		default:
			cur = append(cur, r)
		}
	}
	flush()
	return out
}

func toSnake(s string) string { return strings.Join(words(s), "_") }

func toUpperSnake(s string) string { return strings.ToUpper(toSnake(s)) }

func toUpperCamel(s string) string {
	var b strings.Builder
	for _, w := range words(s) {
		if w == "" {
			continue
		}
		b.WriteString(strings.ToUpper(w[:1]) + w[1:])
	}
	return b.String()
}

// checkName reports XO0101 when name breaks its convention.
func (c *checker) checkName(id *ast.Ident, kind string) {
	if id == nil || id.Name == "" || id.Name == "_" || c.mod == nil || c.mod.IsStd {
		return
	}
	var ok bool
	var want string
	switch kind {
	case "type", "variant", "interface", "type parameter":
		ok, want = isUpperCamel(id.Name), toUpperCamel(id.Name)
	case "constant":
		ok, want = isUpperSnake(id.Name), toUpperSnake(id.Name)
	default:
		ok, want = isSnake(id.Name), toSnake(id.Name)
		if strings.HasPrefix(id.Name, "_") {
			want = "_" + want
		}
	}
	if ok {
		return
	}
	conv := map[string]string{"type": "UpperCamel", "variant": "UpperCamel", "interface": "UpperCamel", "type parameter": "UpperCamel", "constant": "UPPER_SNAKE"}[kind]
	if conv == "" {
		conv = "snake_case"
	}
	d := c.errorf(diag.NamingConvention, id.Sp, "%s name `%s` must be %s", kind, id.Name, conv)
	// Name the habit behind the spelling (decision 0087).
	switch {
	case conv == "snake_case" && id.Name[0] >= 'A' && id.Name[0] <= 'Z' && (kind == "function" || kind == "method" || kind == "field"):
		note(d, token.Span{}, "Go exports a name by capitalizing it; Xo exports with `pub` and spells %ss in snake_case (core 1.5)", kind)
	case conv == "snake_case" && strings.ToLower(id.Name) != id.Name:
		note(d, token.Span{}, "camelCase is the TypeScript, Java, Kotlin, and Swift convention; Xo spells %ss in snake_case (core 1.5)", kind)
	case (kind == "type" || kind == "interface") && id.Name[0] >= 'a' && id.Name[0] <= 'z':
		note(d, token.Span{}, "Go marks an unexported type with a lowercase letter; Xo types are UpperCamel, and only `pub` exports (core 1.5)")
	}
	if want != "" && want != id.Name {
		fix(d, fmt.Sprintf("rename to `%s`", want), replaceEdit(id.Sp, want))
		switch kind {
		case "variable", "parameter", "receiver", "constant":
			if d != nil {
				c.renames = append(c.renames, pendingRename{decl: id, diag: len(c.diags.Items) - 1, want: want})
			}
		}
	}
}

// pendingRename is an XO0101 rename of a local name.
type pendingRename struct {
	decl *ast.Ident
	diag int // index in c.diags.Items
	want string
}

// finishRenames extends each local rename fix to every use of the name, so
// the fix keeps the program meaning the same.
func (c *checker) finishRenames() {
	for _, r := range c.renames {
		if r.diag >= len(c.diags.Items) {
			continue
		}
		d := &c.diags.Items[r.diag]
		if d.Code != string(diag.NamingConvention) || len(d.Fixes) == 0 || d.Span != diag.FromSpan(r.decl.Sp) {
			continue
		}
		v := c.identObj[r.decl]
		if v == nil {
			for _, w := range c.identObj {
				if w.Span == r.decl.Sp {
					v = w
					break
				}
			}
		}
		if v == nil {
			continue
		}
		var uses []*ast.Ident
		for id, w := range c.identObj {
			if w == v && id != r.decl && id.Sp != r.decl.Sp && id.Name == r.decl.Name {
				uses = append(uses, id)
			}
		}
		sort.Slice(uses, func(i, j int) bool { return uses[i].Sp.Start.Offset < uses[j].Sp.Start.Offset })
		for _, id := range uses {
			d.Fixes[0].Edits = append(d.Fixes[0].Edits, replaceEdit(id.Sp, r.want))
		}
	}
}

// ---------------------------------------------------------------------------
// Suggestions (XO0601)

func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

// nearest returns up to three candidates close to name, best first.
func nearest(name string, cands []string) []string {
	type sc struct {
		s string
		d int
	}
	var best []sc
	seen := map[string]bool{}
	limit := max(1, min(3, len(name)/3+1))
	lower := strings.ToLower(name)
	for _, cnd := range cands {
		if cnd == name || seen[cnd] || cnd == "" {
			continue
		}
		seen[cnd] = true
		d := editDistance(lower, strings.ToLower(cnd))
		if strings.ToLower(cnd) == lower {
			d = 0
		}
		if d <= limit {
			best = append(best, sc{cnd, d})
		}
	}
	for i := 1; i < len(best); i++ {
		for j := i; j > 0 && (best[j].d < best[j-1].d || best[j].d == best[j-1].d && best[j].s < best[j-1].s); j-- {
			best[j], best[j-1] = best[j-1], best[j]
		}
	}
	var out []string
	for i := 0; i < len(best) && i < 3; i++ {
		out = append(out, best[i].s)
	}
	return out
}

// confidentGuess reports whether the best edit distance suggestion for name
// is safe to apply as a machine fix: a case only difference, or a unique
// closest candidate at most half the name's length away, for names of
// three or more characters.
func confidentGuess(name string, sugg []string) bool {
	best := sugg[0]
	if strings.EqualFold(best, name) {
		return true
	}
	d := editDistance(strings.ToLower(name), strings.ToLower(best))
	if len(name) < 3 || d*2 > len(name) {
		return false
	}
	if len(sugg) > 1 && editDistance(strings.ToLower(name), strings.ToLower(sugg[1])) == d {
		return false
	}
	return true
}

// unresolved reports XO0601 for name at id with suggestions from cands.
func (c *checker) unresolved(sp token.Span, what, name string, cands []string) *diag.Diagnostic {
	msg := fmt.Sprintf("%s `%s` not found", what, name)
	if i := strings.Index(what, " of "); i > 0 {
		msg = fmt.Sprintf("%s has no %s `%s`", what[i+4:], what[:i], name)
	}
	d := c.errorf(diag.UnresolvedName, sp, "%s", msg)
	if d == nil {
		return nil
	}
	sugg := nearest(name, cands)
	if len(sugg) > 0 {
		note(d, token.Span{}, "did you mean `%s`?", strings.Join(sugg, "`, `"))
		// `xo fix` applies the first fix blindly, so a guess becomes a fix
		// only when it is clear: one closest candidate, and an edit small
		// next to the name (`t` is not `it`, `map` is not `has`).
		if confidentGuess(name, sugg) {
			fix(d, fmt.Sprintf("replace with `%s`", sugg[0]), replaceEdit(sp, sugg[0]))
		}
	}
	return d
}

type typeArgVar struct {
	v     *Var
	param string
	fn    string
	span  token.Span
}

// reportAmbiguous reports XO0402 for type parameters that nothing
// determined: not the arguments, the expected type, or later use.
func (c *checker) reportAmbiguous() {
	for _, t := range c.neverResults {
		if isOpenVar(t) {
			c.unify(t, c.named("Never"))
		}
	}
	done := map[token.Span]bool{}
	for _, tv := range c.typeArgVars {
		if tv.v.Ref != nil || done[tv.span] {
			continue
		}
		done[tv.span] = true
		d := c.errorf(diag.AmbiguousTypeArgs, tv.span, "cannot infer type argument `%s` of `%s`", tv.param, tv.fn)
		if d != nil {
			note(d, token.Span{}, "write it explicitly: `%s[T](...)`, or give the result an expected type (core 2.7)", tv.fn)
		}
	}
}
