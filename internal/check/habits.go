package check

// Habit diagnostics in the checker (decision 0087). Names, calls, types,
// and methods from Go, Rust, TypeScript, Python, Swift, and Kotlin that do
// not resolve in Xo keep their code (XO0601: the name does not resolve)
// but name the habit and the Xo form in a note, with a machine fix when
// the rewrite is unambiguous. They replace the edit distance guess, which
// for these names is usually wrong (`Vec` is not `Eq`). The checker then
// continues with the Xo meaning, so one habit gives one diagnostic.

import (
	"fmt"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/token"
)

// srcOf returns the source text of sp in the current file.
func (c *checker) srcOf(sp token.Span) string {
	if c.file == nil || sp.Start.Offset < 0 || sp.End.Offset > len(c.file.Src) || sp.Start.Offset > sp.End.Offset {
		return ""
	}
	return string(c.file.Src[sp.Start.Offset:sp.End.Offset])
}

// operandSrc is the source of e, parenthesized when a postfix would bind
// to part of it.
func (c *checker) operandSrc(e ast.Expr) string {
	t := c.srcOf(e.Span())
	switch e.(type) {
	case *ast.Ident, *ast.SelectorExpr, *ast.CallExpr, *ast.IndexExpr, *ast.ParenExpr,
		*ast.StringLit, *ast.ListLit, *ast.MapLit, *ast.SetLit, *ast.TupleExpr, *ast.StructLit:
		return t
	}
	return "(" + t + ")"
}

// operandAt parenthesizes text, which replaces the source at sp, when a
// postfix (`.m()`, `[i]`, `?`) or a binary operator follows sp, so the
// rewrite keeps its grouping.
func (c *checker) operandAt(sp token.Span, text string) string {
	if c.file == nil {
		return text
	}
	src := c.file.Src
	i := sp.End.Offset
	for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
		i++
	}
	if i < len(src) && strings.IndexByte(".[?*/%+-<>=!&|", src[i]) >= 0 {
		return "(" + text + ")"
	}
	return text
}

// srcFor returns the source of the file at path among the checked
// modules.
func (c *checker) srcFor(path string) []byte {
	for _, m := range c.order {
		for _, f := range m.Files {
			if f.Path == path {
				return f.Src
			}
		}
	}
	return nil
}

// dollarHabit explains an unused binding that a Kotlin or shell style
// `"$name"` template meant to read: in Xo `$` alone is literal text. It
// adds the note and a fix writing `${name}`, and reports whether it did.
func (c *checker) dollarHabit(d *diag.Diagnostic, v *VarObj) bool {
	src := c.srcFor(v.Span.File)
	if src == nil || v.Name == "" || v.Name[0] == '_' {
		return false
	}
	// Search the rest of the enclosing declaration: up to the next line
	// that starts a top level declaration.
	end := len(src)
	for _, kw := range []string{"\nfn ", "\npub ", "\ntest ", "\nstruct ", "\nenum "} {
		if i := strings.Index(string(src[v.Span.End.Offset:]), kw); i >= 0 && v.Span.End.Offset+i < end {
			end = v.Span.End.Offset + i
		}
	}
	text := string(src[v.Span.End.Offset:end])
	pat := "$" + v.Name
	var edits []diag.Edit
	for i := 0; ; {
		j := strings.Index(text[i:], pat)
		if j < 0 {
			break
		}
		at := i + j
		after := at + len(pat)
		if (after < len(text) && isIdentByte(text[after])) || (at > 0 && text[at-1] == '\\') {
			i = after
			continue
		}
		off := v.Span.End.Offset + at
		start := posFrom(src, v.Span.End, off)
		stop := posFrom(src, v.Span.End, off+len(pat))
		edits = append(edits, replaceEdit(token.Span{File: v.Span.File, Start: start, End: stop}, "${"+v.Name+"}"))
		i = after
	}
	if len(edits) == 0 {
		return false
	}
	note(d, token.Span{}, "`\"$%s\"` is Kotlin and shell interpolation; in an Xo string `$` alone is literal text, so the value is never read: write `${%s}` (core 1.6)", v.Name, v.Name)
	fix(d, "write `${"+v.Name+"}`", edits...)
	return true
}

func isIdentByte(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// posFrom is the position of offset off, counting from pos.
func posFrom(src []byte, pos token.Pos, off int) token.Pos {
	for i := pos.Offset; i < off && i < len(src); i++ {
		if src[i] == '\n' {
			pos.Line++
			pos.Col = 1
		} else {
			pos.Col++
		}
	}
	pos.Offset = off
	return pos
}

// safeNavFix adds a fix to the XO0205 of a one step optional chain,
// `u?.name` with nothing chained after it: `u.map(fn(u) { u.name })`.
func (c *checker) safeNavFix(x *ast.SelectorExpr) {
	tx, ok := x.X.(*ast.TryExpr)
	if !ok || !c.safeNavs[tx] {
		return
	}
	id, ok := tx.X.(*ast.Ident)
	i, have := c.safeNavDiag[tx]
	if !ok || !have || i >= len(c.diags.Items) {
		return
	}
	src := c.file.Src
	if e := x.Sp.End.Offset; e < len(src) && strings.IndexByte(".([?", src[e]) >= 0 {
		return
	}
	d := &c.diags.Items[i]
	if d.Code != string(diag.TryNeedsFailing) || len(d.Fixes) > 0 {
		return
	}
	fix(d, "write `.map(...)`", replaceEdit(x.Sp, id.Name+".map(fn("+id.Name+") { "+id.Name+"."+x.Sel.Name+" })"))
}

// safeNav reports whether e is, or starts with, a TypeScript, Kotlin, or
// Swift optional chain `x?.f` that was reported as XO0205.
func (c *checker) safeNav(e ast.Expr) bool {
	for {
		switch x := e.(type) {
		case *ast.TryExpr:
			if c.safeNavs[x] {
				return true
			}
			e = x.X
		case *ast.SelectorExpr:
			e = x.X
		case *ast.CallExpr:
			e = x.Fun
		case *ast.IndexExpr:
			e = x.X
		case *ast.ParenExpr:
			e = x.X
		default:
			return false
		}
	}
}

// habitNote attaches the habit note, replacing the edit distance guess
// that c.unresolved added.
func habitNote(d *diag.Diagnostic, format string, args ...any) {
	if d == nil {
		return
	}
	d.Notes = d.Notes[:0]
	d.Fixes = d.Fixes[:0]
	d.Notes = append(d.Notes, diag.NoteItem{Message: fmt.Sprintf(format, args...)})
}

// receiverName returns the receiver name of the method being checked
// (through closures), or "".
func (c *checker) receiverName() string {
	for x := c.ctx; x != nil; x = x.parent {
		if x.fn != nil && x.fn.Decl != nil && x.fn.Decl.Recv != nil && x.fn.Decl.Recv.Name != nil {
			return x.fn.Decl.Recv.Name.Name
		}
	}
	return ""
}

// outerFn returns the declared function being checked (through closures).
func (c *checker) outerFn() *FuncObj {
	for x := c.ctx; x != nil; x = x.parent {
		if x.fn != nil {
			return x.fn
		}
	}
	return nil
}

// nullNames are the absent value literals of other languages.
var nullNames = map[string]string{
	"nil":       "Go and Swift",
	"null":      "TypeScript, Kotlin, and Java",
	"undefined": "TypeScript",
	"NULL":      "C",
	"nullptr":   "C++",
	"Nil":       "Ruby",
}

// habitName handles an unresolved value name. ok is false when the name is
// not a known habit.
func (c *checker) habitName(x *ast.Ident, hint Type) (Type, bool) {
	name := x.Name
	if lang, ok := nullNames[name]; ok {
		d := c.errorf(diag.UnresolvedName, x.Sp, "name `%s` not found", name)
		habitNote(d, "`%s` is %s; Xo has no null: a value that may be absent is `T?`, written `None` when absent and handled with `?`, `??`, `if let`, or `match`; a failure is `Err(e)` in a function declared `-> T ! E` (core 2.5, 2.6)", name, lang)
		if hint != nil && c.optionElem(hint) != nil {
			fix(d, "write `None`", replaceEdit(x.Sp, "None"))
			return hint, true
		}
		return badType, true
	}
	switch name {
	case "this", "self":
		recv := c.receiverName()
		d := c.errorf(diag.UnresolvedName, x.Sp, "name `%s` not found", name)
		lang := "TypeScript, Kotlin, and Java"
		if name == "self" {
			lang = "Python, Rust, and Swift"
		}
		if recv != "" && recv != name {
			habitNote(d, "`%s` is the %s receiver; an Xo method names its receiver, here `%s` (core 3.4)", name, lang, recv)
			fix(d, "write `"+recv+"`", replaceEdit(x.Sp, recv))
			if v := c.lookupLocal(recv); v != nil {
				return c.useVar(&ast.Ident{Base: x.Base, Name: recv}, v), true
			}
			return badType, true
		}
		habitNote(d, "`%s` is the %s receiver; Xo methods name their receiver, `fn (p: P) name()`, and plain functions have none (core 3.4)", name, lang)
		return badType, true
	case "pass":
		// Python's empty statement; a variable named `pass` resolves first.
		d := c.errorf(diag.UnresolvedName, x.Sp, "name `%s` not found", name)
		habitNote(d, "`pass` is Python's empty statement; an empty Xo block is `{}`, and the unit value is `()` (core 3.2)")
		fix(d, "write `()`", replaceEdit(x.Sp, "()"))
		return unitType, true
	case "True", "False":
		d := c.errorf(diag.UnresolvedName, x.Sp, "name `%s` not found", name)
		habitNote(d, "`%s` is Python syntax; Xo writes `%s` (core 1.6)", name, strings.ToLower(name))
		fix(d, "write `"+strings.ToLower(name)+"`", replaceEdit(x.Sp, strings.ToLower(name)))
		return c.named("Bool"), true
	}
	return nil, false
}

// printNames maps print functions of other languages to whether they end
// the line ("" for no machine fix).
var printNames = map[string]string{
	"println": "println", "print": "", "printf": "", "puts": "println", "echo": "println",
	"fmt.Println": "println", "fmt.Print": "print", "fmt.Printf": "", "fmt.Sprintf": "",
	"console.log": "println", "console.error": "eprintln", "System.out.println": "println",
	"eprintln": "eprintln",
}

// habitCall handles a call of an unresolved function name.
func (c *checker) habitCall(x *ast.CallExpr, id *ast.Ident, hint Type) (Type, bool) {
	name := id.Name
	args := x.Args
	argSrc := func(i int) string { return c.srcOf(args[i].Value.Span()) }
	report := func(format string, a ...any) *diag.Diagnostic {
		d := c.errorf(diag.UnresolvedName, id.Sp, "function `%s` not found", name)
		habitNote(d, format, a...)
		return d
	}
	if _, ok := printNames[name]; ok {
		return c.habitPrint(x, name, id.Sp, hint)
	}
	switch name {
	case "len", "count", "size":
		if len(args) != 1 {
			return nil, false
		}
		lang := map[string]string{"len": "Go and Python", "count": "Swift", "size": "C++"}[name]
		d := report("`%s(x)` is %s; Xo asks the value: `x.len()` (spec/std.md 2)", name, lang)
		fix(d, "write `.len()`", replaceEdit(x.Sp, c.operandSrc(args[0].Value)+".len()"))
		c.infer(args[0].Value, nil)
		return c.named("Int"), true
	case "append":
		if len(args) < 2 {
			return nil, false
		}
		var elems []string
		for i := 1; i < len(args); i++ {
			elems = append(elems, argSrc(i))
		}
		d := report("`append(xs, v)` is Go; an Xo list is a value: `xs + [v]` is the longer list, and `xs.push(v)` changes a `var` list in place (spec/std.md 2)")
		fix(d, "write `xs + [...]`", replaceEdit(x.Sp, c.operandAt(x.Sp, c.operandSrc(args[0].Value)+" + ["+strings.Join(elems, ", ")+"]")))
		t := c.infer(args[0].Value, hint)
		for i := 1; i < len(args); i++ {
			c.infer(args[i].Value, c.listElem(t))
		}
		return t, true
	case "range":
		if len(args) < 1 || len(args) > 2 {
			return nil, false
		}
		lo, hi := "0", argSrc(0)
		var loE ast.Expr = &ast.IntLit{Base: ast.Base{Sp: x.Sp}, Raw: "0", Value: "0"}
		hiE := args[0].Value
		if len(args) == 2 {
			lo, hi = argSrc(0), argSrc(1)
			loE, hiE = args[0].Value, args[1].Value
		}
		d := report("`range(n)` is Python; Xo writes the range itself, `%s..%s`, which excludes the upper bound as `range` does (core 3.5)", lo, hi)
		fix(d, "write `"+lo+".."+hi+"`", replaceEdit(x.Sp, c.operandAt(x.Sp, lo+".."+hi)))
		return c.infer(&ast.RangeExpr{Base: ast.Base{Sp: x.Sp}, Lo: loE, Hi: hiE}, hint), true
	case "str", "String", "toString":
		if len(args) != 1 {
			return nil, false
		}
		d := report("`%s(x)` converts to text in Python and Kotlin; Xo interpolates any `Display` value: `\"${x}\"` (core 1.6)", name)
		fix(d, "write `\"${...}\"`", replaceEdit(x.Sp, "\"${"+argSrc(0)+"}\""))
		c.infer(args[0].Value, nil)
		return c.named("Str"), true
	case "int", "float":
		if len(args) != 1 {
			return nil, false
		}
		xo := map[string]string{"int": "Int", "float": "Float"}[name]
		t := c.infer(args[0].Value, nil)
		if c.isPrelude(t, "Str") {
			report("`%s(s)` parses text in Python; Xo returns an optional, `%s.parse(s)`, which is `None` for bad input (core 2.1)", name, xo)
			return c.named(xo), true
		}
		d := report("`%s(x)` is Python; Xo converts with the type name, `%s(x)` (core 2.1)", name, xo)
		fix(d, "write `"+xo+"(...)`", replaceEdit(id.Sp, xo))
		return c.named(xo), true
	case "sorted", "reversed", "abs":
		if len(args) != 1 {
			return nil, false
		}
		m := map[string]string{"sorted": "sort", "reversed": "reverse", "abs": "abs"}[name]
		d := report("`%s(x)` is Python; Xo calls the method, `x.%s()` (spec/std.md 2)", name, m)
		fix(d, "write `."+m+"()`", replaceEdit(x.Sp, c.operandSrc(args[0].Value)+"."+m+"()"))
		return c.infer(args[0].Value, hint), true
	case "min", "max":
		if len(args) != 2 {
			return nil, false
		}
		d := report("`%s(a, b)` is Python and Go; Xo calls the method on an `Ord` value, `a.%s(b)` (spec/std.md 1)", name, name)
		fix(d, "write `a."+name+"(b)`", replaceEdit(x.Sp, c.operandSrc(args[0].Value)+"."+name+"("+argSrc(1)+")"))
		t := c.infer(args[0].Value, hint)
		c.check(args[1].Value, t)
		return t, true
	case "panic":
		if len(args) != 1 {
			return nil, false
		}
		d := report("`panic(msg)` is Go; Xo stops the program on a broken invariant with `fault(msg)`, and reports expected failures with `Err(e)` (core 8, 2.6)")
		t := c.infer(args[0].Value, nil)
		if c.isPrelude(t, "Str") {
			fix(d, "write `fault`", replaceEdit(id.Sp, "fault"))
		}
		return c.named("Never"), true
	case "delete":
		if len(args) != 2 {
			return nil, false
		}
		d := report("`delete(m, k)` is Go; Xo calls `m.remove(k)` on a `var` map (spec/std.md 2)")
		fix(d, "write `m.remove(k)`", replaceEdit(x.Sp, c.operandSrc(args[0].Value)+".remove("+argSrc(1)+")"))
		c.inferArgs(x)
		return unitType, true
	case "map", "filter":
		if len(args) != 2 {
			return nil, false
		}
		d := report("`%s(f, xs)` is Python; Xo calls the method on the collection, `xs.%s(f)` (spec/std.md 2)", name, name)
		recv := c.operandSrc(args[1].Value)
		var on ast.Expr = args[1].Value
		if c.isPrelude(c.infer(args[1].Value, nil), "Range") {
			// A range has no map or filter: make it a list first.
			recv += ".to_list()"
			on = &ast.CallExpr{Base: args[1].Base, Fun: &ast.SelectorExpr{Base: id.Base, X: args[1].Value, Sel: &ast.Ident{Base: id.Base, Name: "to_list"}}}
		}
		fix(d, "write `xs."+name+"(f)`", replaceEdit(x.Sp, recv+"."+name+"("+argSrc(0)+")"))
		call := &ast.CallExpr{Base: x.Base, Fun: &ast.SelectorExpr{Base: id.Base, X: on, Sel: &ast.Ident{Base: id.Base, Name: name}}, Args: args[:1]}
		return c.infer(call, hint), true
	case "list", "set", "enumerate":
		if len(args) != 1 {
			return nil, false
		}
		t := c.infer(args[0].Value, nil)
		r := resolve(t)
		n, _ := r.(*Named)
		kind := ""
		if n != nil && n.Obj.Module == c.prelude {
			kind = n.Obj.Name
		}
		m := map[string]string{"list": "to_list", "set": "to_set", "enumerate": "indexed"}[name]
		d := report("`%s(x)` is Python; Xo calls the method on the value, `x.%s()` (spec/std.md 2)", name, m)
		switch {
		case name == "list" && kind == "List":
			fix(d, "remove `list(...)`", replaceEdit(x.Sp, argSrc(0)))
			return t, true
		case kind == "":
			return badType, true
		}
		fix(d, "write `."+m+"()`", replaceEdit(x.Sp, c.operandSrc(args[0].Value)+"."+m+"()"))
		call := &ast.CallExpr{Base: x.Base, Fun: &ast.SelectorExpr{Base: id.Base, X: args[0].Value, Sel: &ast.Ident{Base: id.Base, Name: m}}}
		return c.callMethod(t, call.Fun.(*ast.SelectorExpr), nil, call, hint), true
	case "sum":
		if len(args) != 1 {
			return nil, false
		}
		t := c.infer(args[0].Value, nil)
		zero := "0"
		if e := c.listElem(t); e != nil && c.isPrelude(e, "Float") {
			zero = "0.0"
		}
		d := report("`sum(xs)` is Python; Xo folds the list, `xs.fold(%s, fn(a, b) { a + b })` (spec/std.md 2)", zero)
		fix(d, "write `.fold(...)`", replaceEdit(x.Sp, c.operandSrc(args[0].Value)+".fold("+zero+", fn(a, b) { a + b })"))
		if e := c.listElem(t); e != nil {
			return e, true
		}
		return badType, true
	case "make", "new":
		report("`%s(...)` is Go; Xo writes the value itself: `[]` for an empty list, `{}` for an empty map, `Chan[T].new(n)` for a channel, and `T{field: v}` for a struct (core 1.6, 7.3)", name)
		return badType, true
	case "assert":
		report("`assert(c)` is Python, Swift, and C; an Xo test checks with `expect(c)`, and a function states conditions with `requires` and `ensures` (core 9.1, 9.2)")
		c.inferArgs(x)
		return unitType, true
	case "typeof", "type", "isinstance":
		report("`%s` inspects types at run time in TypeScript and Python; Xo types are known when checking: use an enum and `match` for values of several shapes (core 2.3)", name)
		c.inferArgs(x)
		return badType, true
	}
	return nil, false
}

// listElem returns T for List[T], or nil.
func (c *checker) listElem(t Type) Type {
	if n, ok := resolve(t).(*Named); ok && n.Obj.Module == c.prelude && n.Obj.Name == "List" {
		return n.Args[0]
	}
	return nil
}

// habitModules are module-like names whose members are calls from another
// language's standard library.
var habitModules = map[string]string{
	"fmt": "Go", "console": "TypeScript", "System": "Java and Kotlin", "Math": "TypeScript and Java", "String": "Rust",
	"math": "Go and Python", "strings": "Go", "strconv": "Go", "errors": "Go", "time": "Go and Python",
}

// habitModuleCall handles `fmt.Println(...)`, `console.log(...)`,
// `Math.max(a, b)`, `strings.ToUpper(s)` and the like when the module name
// does not resolve.
func (c *checker) habitModuleCall(x *ast.CallExpr, mod *ast.Ident, sel *ast.SelectorExpr, hint Type) (Type, bool) {
	lang, ok := habitModules[mod.Name]
	if !ok {
		return nil, false
	}
	full := mod.Name + "." + sel.Sel.Name
	if full == "String.from" || full == "String.new" {
		d := c.errorf(diag.UnresolvedName, mod.Sp, "name `%s` not found", mod.Name)
		if full == "String.new" && len(x.Args) == 0 {
			habitNote(d, "`String::new()` is Rust; an empty Xo string is `\"\"`, and a `Str` is immutable (core 1.6)")
			fix(d, "write `\"\"`", replaceEdit(x.Sp, `""`))
			return c.named("Str"), true
		}
		habitNote(d, "`String::from(s)` is Rust; a string literal already is a `Str`, and other values interpolate, `\"${x}\"` (core 1.6)")
		if len(x.Args) == 1 {
			t := c.infer(x.Args[0].Value, nil)
			if c.isPrelude(t, "Str") {
				fix(d, "remove `String::from`", replaceEdit(x.Sp, c.srcOf(x.Args[0].Value.Span())))
			} else {
				fix(d, "write `\"${...}\"`", replaceEdit(x.Sp, "\"${"+c.srcOf(x.Args[0].Value.Span())+"}\""))
			}
			return c.named("Str"), true
		}
		c.inferArgs(x)
		return c.named("Str"), true
	}
	if mod.Name == "String" {
		return nil, false
	}
	if mod.Name == "System" {
		full = c.srcOf(x.Fun.Span())
	}
	if _, ok := printNames[full]; ok {
		return c.habitPrint(x, full, x.Fun.Span(), hint)
	}
	d := c.errorf(diag.UnresolvedName, mod.Sp, "name `%s` not found", mod.Name)
	method := map[string]string{
		"strings.ToUpper": "to_upper", "strings.ToLower": "to_lower", "strings.TrimSpace": "trim",
		"strings.Contains": "contains", "strings.HasPrefix": "starts_with", "strings.HasSuffix": "ends_with",
		"strings.Split": "split", "strings.Repeat": "repeat", "strings.TrimPrefix": "trim_prefix",
		"strings.TrimSuffix": "trim_suffix", "strings.Join": "join", "strings.ReplaceAll": "replace",
		"math.Abs": "abs", "Math.abs": "abs", "math.Sqrt": "sqrt", "Math.sqrt": "sqrt",
		"math.Floor": "floor", "Math.floor": "floor", "math.Ceil": "ceil", "Math.ceil": "ceil",
		"Math.round": "round", "math.Round": "round", "math.Max": "max", "Math.max": "max", "math.Min": "min", "Math.min": "min",
		"math.sqrt": "sqrt", "math.floor": "floor", "math.ceil": "ceil",
	}[full]
	if method != "" && len(x.Args) >= 1 {
		rest := ""
		var restArgs []string
		for _, a := range x.Args[1:] {
			restArgs = append(restArgs, c.srcOf(a.Value.Span()))
		}
		rest = strings.Join(restArgs, ", ")
		recvArg := x.Args[0].Value
		if full == "strings.Join" && len(x.Args) == 2 {
			// strings.Join(xs, sep) is xs.join(sep): the receiver is the list.
			recvArg = x.Args[0].Value
		}
		habitNote(d, "`%s` is %s's library; Xo calls the method on the value: `%s.%s(%s)` (spec/std.md 2)", full, lang, c.operandSrc(recvArg), method, rest)
		fix(d, "write `."+method+"(...)`", replaceEdit(x.Sp, c.operandSrc(recvArg)+"."+method+"("+rest+")"))
		c.inferArgs(x)
		return badType, true
	}
	habitNote(d, "`%s` is %s's library; Xo has no module `%s`: the std modules are listed in spec/std.md, and most text, number, and collection operations are methods on the value", full, lang, mod.Name)
	c.inferArgs(x)
	return badType, true
}

// habitPrint handles print functions. Xo prints through the `Stdio`
// capability, so the fix needs an `Os` or `Stdio` parameter in scope.
func (c *checker) habitPrint(x *ast.CallExpr, name string, at token.Span, hint Type) (Type, bool) {
	what := "function `" + name + "` not found"
	if i := strings.IndexByte(name, '.'); i > 0 {
		what = "name `" + name[:i] + "` not found"
		at = token.Span{File: at.File, Start: at.Start, End: token.Pos{Line: at.Start.Line, Col: at.Start.Col + i, Offset: at.Start.Offset + i}}
	}
	d := c.errorf(diag.UnresolvedName, at, "%s", what)
	habitNote(d, "`%s` prints from anywhere in other languages; Xo prints through the `Stdio` capability, `os.stdio.println(\"${x}\")` in `main` or a `stdio: Stdio` parameter elsewhere, so a function's output is visible in its signature; `dbg(x)` prints a value while debugging (spec/std.md 3, core 6.3, 11.4)", name)
	var args []string
	allStr := true
	for _, a := range x.Args {
		t := c.infer(a.Value, nil)
		args = append(args, c.srcOf(a.Value.Span()))
		if !c.isPrelude(t, "Str") {
			allStr = false
		}
	}
	method := printNames[name]
	if method == "" || len(args) == 0 {
		return unitType, true
	}
	// Find a capability to print with.
	recv := ""
	if fn := c.outerFn(); fn != nil {
		for _, p := range fn.Params {
			switch {
			case c.isPrelude(p.Type, "Os"):
				recv = p.Name + ".stdio"
			case c.isPrelude(p.Type, "Stdio"):
				recv = p.Name
			}
			if recv != "" {
				break
			}
		}
	}
	if recv == "" {
		return unitType, true
	}
	var text string
	if len(args) == 1 && allStr {
		text = args[0]
	} else {
		var parts []string
		for _, a := range args {
			parts = append(parts, "${"+a+"}")
		}
		text = "\"" + strings.Join(parts, " ") + "\""
	}
	fix(d, "write `"+recv+"."+method+"(...)`", replaceEdit(x.Sp, recv+"."+method+"("+text+")"))
	return unitType, true
}

// typeHabits maps type names of other languages to the Xo type. A value
// starting with "!" is a message without a fix.
var typeHabits = map[string]struct{ lang, xo string }{
	"String": {"Rust, Kotlin, Swift, and Java", "Str"}, "string": {"Go and TypeScript", "Str"}, "str": {"Python and Rust", "Str"},
	"int": {"Go, Python, and C", "Int"}, "i64": {"Rust", "Int"}, "int64": {"Go", "Int"}, "Integer": {"Java", "Int"}, "Long": {"Kotlin and Java", "Int"},
	"isize": {"Rust", "Int"}, "usize": {"Rust", "Int"}, "number": {"TypeScript", "Float"}, "Number": {"TypeScript", "Float"},
	"i32": {"Rust", "Int32"}, "int32": {"Go", "Int32"}, "i16": {"Rust", "Int16"}, "int16": {"Go", "Int16"}, "i8": {"Rust", "Int8"}, "int8": {"Go", "Int8"},
	"u8": {"Rust", "UInt8"}, "uint8": {"Go", "UInt8"}, "byte": {"Go", "UInt8"}, "Byte": {"Kotlin", "UInt8"},
	"u16": {"Rust", "UInt16"}, "uint16": {"Go", "UInt16"}, "u32": {"Rust", "UInt32"}, "uint32": {"Go", "UInt32"},
	"u64": {"Rust", "UInt64"}, "uint64": {"Go", "UInt64"}, "uint": {"Go", "UInt64"},
	"f64": {"Rust", "Float"}, "float64": {"Go", "Float"}, "float": {"Python and C", "Float"}, "double": {"C and Java", "Float"}, "Double": {"Kotlin and Swift", "Float"},
	"f32": {"Rust", "Float32"}, "float32": {"Go", "Float32"},
	"bool": {"Go, Rust, and Python", "Bool"}, "boolean": {"TypeScript", "Bool"}, "Boolean": {"Kotlin and Java", "Bool"},
	"char": {"Rust", "Rune"}, "rune": {"Go", "Rune"}, "Character": {"Swift", "Rune"}, "Char": {"Kotlin", "Rune"},
	"Vec": {"Rust", "List"}, "Array": {"TypeScript and Swift", "List"}, "ArrayList": {"Java and Kotlin", "List"}, "MutableList": {"Kotlin", "List"}, "list": {"Python", "List"},
	"HashMap": {"Rust and Java", "Map"}, "BTreeMap": {"Rust", "Map"}, "Dictionary": {"Swift", "Map"}, "dict": {"Python", "Map"}, "MutableMap": {"Kotlin", "Map"}, "Record": {"TypeScript", "Map"},
	"HashSet": {"Rust and Java", "Set"}, "BTreeSet": {"Rust", "Set"}, "MutableSet": {"Kotlin", "Set"}, "set": {"Python", "Set"}, "frozenset": {"Python", "Set"},
	"tuple": {"Python", ""}, "Optional": {"Python and Java", ""}, "Maybe": {"Haskell", ""},
	"error": {"Go", "Error"}, "Exception": {"Python, Java, and Kotlin", "Error"}, "Throwable": {"Java and Kotlin", "Error"},
	"void": {"TypeScript and C", "()"}, "Void": {"Swift", "()"}, "Unit": {"Kotlin", "()"}, "NoneType": {"Python", "()"},
	"Box": {"Rust", "*"}, "Rc": {"Rust", "*"}, "Arc": {"Rust", "*"}, "RefCell": {"Rust", "!"},
	"any": {"TypeScript and Go", "!"}, "Any": {"Kotlin and Swift", "!"}, "object": {"Python and TypeScript", "!"}, "Object": {"Java", "!"}, "unknown": {"TypeScript", "!"},
	"Self": {"Rust and Swift", "!"}, "Hashable": {"Swift", "Hash"}, "Comparable": {"Swift and Kotlin", "Ord"}, "Equatable": {"Swift", "Eq"},
	"Codable": {"Swift", "Json"}, "Serialize": {"Rust", "Json"}, "Display": {"", ""},
}

// habitType handles an unresolved type name. It returns the recovered Xo
// type; ok is false when the name is not a known habit.
func (c *checker) habitType(x *ast.NamedType, env *typeEnv) (Type, bool) {
	name := x.Path[0].Name
	h, ok := typeHabits[name]
	if !ok || h.lang == "" {
		return nil, false
	}
	var d *diag.Diagnostic
	if !env.quiet {
		d = c.errorf(diag.UnresolvedName, x.Path[0].Sp, "type `%s` not found", name)
	}
	argText := func() string {
		var out []string
		for _, a := range x.Args {
			out = append(out, c.srcOf(a.Span()))
		}
		return strings.Join(out, ", ")
	}
	switch {
	case h.xo == "*":
		habitNote(d, "`%s[T]` is a Rust pointer type; Xo values need no box: every value is shared or copied as is, so write `T` (core 5.1)", name)
		if len(x.Args) == 1 {
			fix(d, "write the inner type", replaceEdit(x.Sp, c.srcOf(x.Args[0].Span())))
			return c.resolveType(x.Args[0], env), true
		}
		return badType, true
	case h.xo == "!":
		switch name {
		case "Self":
			habitNote(d, "`Self` is %s syntax; Xo has no `Self` type: name the type itself (core 3.4)", h.lang)
		case "RefCell":
			habitNote(d, "`RefCell` is Rust interior mutability; Xo shares mutable state with `Mutex[T]` or `Atomic[T]`, and a `var` binding changes a value in place (core 5.4)")
		default:
			habitNote(d, "`%s` is the %s top type; Xo has none: make the function generic, `fn f[T](x: T)`, or name an interface with the methods you need (core 2.4, 2.7)", name, h.lang)
		}
		return badType, true
	case h.xo == "":
		switch name {
		case "tuple":
			habitNote(d, "`tuple[A, B]` is Python; Xo writes a tuple type `(A, B)` (core 2.1)")
			if len(x.Args) > 0 {
				fix(d, "write `(...)`", replaceEdit(x.Sp, "("+argText()+")"))
				var elems []Type
				for _, a := range x.Args {
					elems = append(elems, c.resolveType(a, env))
				}
				return &Tuple{Elems: elems}, true
			}
		default:
			habitNote(d, "`%s[T]` is %s; Xo writes an optional as `T?` (core 2.5)", name, h.lang)
			if len(x.Args) == 1 {
				fix(d, "write `T?`", replaceEdit(x.Sp, c.srcOf(x.Args[0].Span())+"?"))
				return c.optionOf(c.resolveType(x.Args[0], env)), true
			}
		}
		return badType, true
	case h.xo == "()":
		habitNote(d, "`%s` is the %s empty result; Xo writes `()`, and a function returning nothing leaves out `-> T` (core 2.1, 3.2)", name, h.lang)
		fix(d, "write `()`", replaceEdit(x.Sp, "()"))
		return unitType, true
	}
	habitNote(d, "`%s` is the %s name; Xo's is `%s` (core 2.1)", name, h.lang, h.xo)
	fix(d, "write `"+h.xo+"`", replaceEdit(x.Path[0].Sp, h.xo))
	nt := &ast.NamedType{Base: x.Base, Path: []*ast.Ident{{Base: x.Path[0].Base, Name: h.xo}}, Args: x.Args}
	return c.resolveNamedType(nt, env), true
}

// methodHabits maps method names of other languages to the Xo method,
// with the languages they come from.
var methodHabits = map[string]struct{ lang, xo string }{
	"length": {"TypeScript and Java", "len"}, "size": {"Kotlin and Java", "len"}, "count": {"Swift", "len"},
	"append": {"Python and Swift", "push"}, "add": {"Kotlin and Java", "push"}, "push_back": {"C++", "push"},
	"includes": {"TypeScript", "contains"}, "contains_key": {"Rust", "contains"}, "containsKey": {"Kotlin and Java", "contains"},
	"has_key": {"Python 2", "contains"}, "has": {"TypeScript", "contains"},
	"toUpperCase": {"TypeScript and Java", "to_upper"}, "toLowerCase": {"TypeScript and Java", "to_lower"},
	"upper": {"Python", "to_upper"}, "lower": {"Python", "to_lower"}, "uppercase": {"Kotlin", "to_upper"}, "lowercase": {"Kotlin", "to_lower"},
	"to_uppercase": {"Rust", "to_upper"}, "to_lowercase": {"Rust", "to_lower"},
	"strip": {"Python", "trim"}, "lstrip": {"Python", "trim_start"}, "rstrip": {"Python", "trim_end"},
	"trimStart": {"TypeScript", "trim_start"}, "trimEnd": {"TypeScript", "trim_end"},
	"startswith": {"Python", "starts_with"}, "endswith": {"Python", "ends_with"},
	"hasPrefix": {"Swift", "starts_with"}, "hasSuffix": {"Swift", "ends_with"},
	"items": {"Python", "entries"}, "sorted": {"Swift and Kotlin", "sort"}, "reversed": {"Swift and Kotlin", "reverse"},
	"replaceAll": {"TypeScript", "replace"}, "set": {"TypeScript", "put"},
	"firstOrNull": {"Kotlin", "first"}, "lastOrNull": {"Kotlin", "last"}, "isEmpty": {"Kotlin and Swift", "is_empty"},
}

// dropMethods are Rust and Java conversions with no Xo meaning: the call
// is removed.
var dropMethods = map[string]string{
	"clone": "Rust", "to_owned": "Rust", "as_str": "Rust", "iter": "Rust", "into_iter": "Rust",
	"borrow": "Rust", "as_ref": "Rust", "collect": "Rust", "copy": "Python", "toList": "Kotlin",
}

// habitMethod handles a method call that does not resolve on a value of
// type r. ok is false when the method is not a known habit.
func (c *checker) habitMethod(bt Type, r Type, sel *ast.SelectorExpr, x *ast.CallExpr, hint Type) (Type, bool) {
	name := sel.Sel.Name
	tname := deep(bt).String()
	report := func() *diag.Diagnostic {
		return c.errorf(diag.UnresolvedName, sel.Sel.Sp, "`%s` has no field or method `%s`", tname, name)
	}
	if lang, ok := dropMethods[name]; ok && len(x.Args) == 0 {
		d := report()
		habitNote(d, "`.%s()` is %s; Xo values are immutable and shared, and collections have their methods directly, so the call is not needed (core 5.1)", name, lang)
		fix(d, "remove `."+name+"()`", replaceEdit(token.Span{File: x.Sp.File, Start: sel.X.Span().End, End: x.Sp.End}, ""))
		return bt, true
	}
	switch name {
	case "unwrap", "get", "orElseThrow":
		if name == "get" && c.optionElem(r) == nil {
			break
		}
		if c.optionElem(r) == nil {
			if _, _, ok := c.resultParts(r); !ok {
				break
			}
		}
		d := report()
		habitNote(d, "`.%s()` panics on a missing value in Rust, Java, and Kotlin; Xo says why the value cannot be missing, `x.or_fault(\"why\")`, or handles the case with `?`, `??`, `if let`, or `match` (core 2.5, spec/std.md 2)", name)
		if t := c.optionElem(r); t != nil {
			return t, true
		}
		t, _, _ := c.resultParts(r)
		return t, true
	case "expect":
		if len(x.Args) != 1 {
			break
		}
		elem := c.optionElem(r)
		if elem == nil {
			if t, _, ok := c.resultParts(r); ok {
				elem = t
			}
		}
		if elem == nil {
			break
		}
		d := report()
		habitNote(d, "`.expect(msg)` is Rust; Xo writes `.or_fault(msg)` (spec/std.md 2)")
		fix(d, "write `.or_fault`", replaceEdit(sel.Sel.Sp, "or_fault"))
		c.check(x.Args[0].Value, c.named("Str"))
		return elem, true
	case "or_err":
		// Option's `or_err` on what is already a Result (a parse that
		// reports why, such as Duration.parse): replace the error
		t, _, ok := c.resultParts(r)
		if !ok || len(x.Args) != 1 || x.Args[0].Name != nil {
			break
		}
		d := report()
		habitNote(d, "`.or_err(e)` turns an optional value into a Result; this is already a Result, so replace its error with `.map_err(fn(_) { e })` (spec/std.md 2.5)")
		arg := c.srcOf(x.Args[0].Value.Span())
		fix(d, "write `.map_err(fn(_) { ... })`", replaceEdit(token.Span{File: x.Sp.File, Start: sel.Sel.Sp.Start, End: x.Sp.End}, "map_err(fn(_) { "+arg+" })"))
		return c.resultOf(t, c.infer(x.Args[0].Value, nil)), true
	case "to_string", "toString", "ToString", "description":
		if len(x.Args) != 0 {
			break
		}
		d := report()
		if c.isPrelude(r, "Str") {
			habitNote(d, "`.%s()` converts to text in Rust, Kotlin, and Java; this is already a `Str`", name)
			fix(d, "remove `."+name+"()`", replaceEdit(token.Span{File: x.Sp.File, Start: sel.X.Span().End, End: x.Sp.End}, ""))
		} else {
			habitNote(d, "`.%s()` converts to text in Rust, Kotlin, and Java; Xo interpolates any `Display` value: `\"${x}\"` (core 1.6)", name)
			fix(d, "write `\"${...}\"`", replaceEdit(x.Sp, "\"${"+c.srcOf(sel.X.Span())+"}\""))
		}
		return c.named("Str"), true
	case "join":
		// Python: ", ".join(xs) is xs.join(", ").
		if !c.isPrelude(r, "Str") || len(x.Args) != 1 {
			break
		}
		d := report()
		habitNote(d, "`sep.join(xs)` is Python; Xo joins from the list, `xs.join(sep)` (spec/std.md 2)")
		fix(d, "write `xs.join(sep)`", replaceEdit(x.Sp, c.operandSrc(x.Args[0].Value)+".join("+c.srcOf(sel.X.Span())+")"))
		c.infer(x.Args[0].Value, nil)
		return c.named("Str"), true
	case "push_str":
		d := report()
		habitNote(d, "`.push_str(s)` is Rust; an Xo `Str` is immutable: `text += s` on a `var` binding, or a `StrBuilder` for many pieces (spec/std.md 2)")
		c.inferArgs(x)
		return unitType, true
	case "forEach", "for_each":
		d := report()
		habitNote(d, "`.%s(f)` is TypeScript, Kotlin, and Rust; Xo loops with `for x in xs { ... }` (core 3.5)", name)
		c.inferArgs(x)
		return unitType, true
	case "substring", "substr", "slice":
		if !c.isPrelude(r, "Str") && c.listElem(r) == nil {
			break
		}
		d := report()
		habitNote(d, "`.%s(a, b)` is TypeScript and Java; Xo slices with a range, `x[a..b]` (core 3.6)", name)
		if len(x.Args) == 2 {
			fix(d, "write `[a..b]`", replaceEdit(token.Span{File: x.Sp.File, Start: sel.X.Span().End, End: x.Sp.End}, "["+c.srcOf(x.Args[0].Value.Span())+".."+c.srcOf(x.Args[1].Value.Span())+"]"))
		}
		c.inferArgs(x)
		return bt, true
	}
	xo := ""
	lang := ""
	if h, ok := methodHabits[name]; ok {
		xo, lang = h.xo, h.lang
	} else if snake := toSnake(name); snake != name {
		xo, lang = snake, "camelCase"
	}
	if xo == "" {
		return nil, false
	}
	if m, _ := c.findMethodAny(r, xo); m == nil {
		return nil, false
	}
	// `count(x)` and `add(x)` on other types mean other things.
	if (name == "count" || name == "size" || name == "length") && len(x.Args) != 0 {
		return nil, false
	}
	d := report()
	if lang == "camelCase" {
		habitNote(d, "`%s` is a camelCase name from TypeScript, Kotlin, Java, or Swift; Xo methods are snake_case: `%s` (core 1.5)", name, xo)
	} else {
		habitNote(d, "`.%s` is the %s name; Xo's is `.%s` (spec/std.md 2)", name, lang, xo)
	}
	fix(d, "write `"+xo+"`", replaceEdit(sel.Sel.Sp, xo))
	call := &ast.CallExpr{Base: x.Base, Fun: &ast.SelectorExpr{Base: sel.Base, X: sel.X, Sel: &ast.Ident{Base: sel.Sel.Base, Name: xo}}, Args: x.Args}
	return c.callMethodQuiet(bt, call, hint), true
}

// goRange reports Go's `for k, v := range x` (XO0177) with the Xo form for
// the type of x, and returns the element type that binds the names the
// way Go would.
func (c *checker) goRange(x *ast.ForStmt, it Type) Type {
	var names []string
	switch p := x.Pattern.(type) {
	case *ast.TuplePat:
		for _, e := range p.Elems {
			names = append(names, c.srcOf(e.Span()))
		}
	default:
		names = []string{c.srcOf(p.Span())}
	}
	header := token.Span{File: x.Sp.File, Start: x.Sp.Start, End: x.Body.Sp.Start}
	if x.Label != nil {
		header.Start = x.Sp.Start
		for i := header.Start.Offset; i < len(c.file.Src) && i < x.Body.Sp.Start.Offset; i++ {
			if strings.HasPrefix(string(c.file.Src[i:]), "for") {
				header.Start = token.Pos{Line: x.Sp.Start.Line, Col: x.Sp.Start.Col + (i - x.Sp.Start.Offset), Offset: i}
				break
			}
		}
	}
	d := c.errorf(diag.ForeignSyntax, token.Span{File: header.File, Start: header.Start, End: token.Pos{Line: header.Start.Line, Col: header.Start.Col + 3, Offset: header.Start.Offset + 3}},
		"`for %s := range %s` is Go syntax; Xo writes `for pattern in %s` (core 3.5)", strings.Join(names, ", "), c.srcOf(x.Iter.Span()), c.srcOf(x.Iter.Span()))
	iter := c.operandSrc(x.Iter)
	r := resolve(it)
	if isBad(r) {
		return badType
	}
	two := len(names) == 2
	first, second := names[0], ""
	if two {
		second = names[1]
	}
	var form string
	var elem Type
	n, _ := r.(*Named)
	kind := ""
	if n != nil && n.Obj.Module == c.prelude {
		kind = n.Obj.Name
	}
	intT := c.named("Int")
	switch {
	case kind == "List" || kind == "Buf":
		switch {
		case two && first == "_":
			form, elem = "for "+second+" in "+iter, &Tuple{Elems: []Type{intT, n.Args[0]}}
		case two:
			form, elem = "for ("+first+", "+second+") in "+iter+".indexed()", &Tuple{Elems: []Type{intT, n.Args[0]}}
		default:
			form, elem = "for "+first+" in 0.."+iter+".len()", intT
		}
	case kind == "Map":
		switch {
		case two && first == "_":
			form, elem = "for "+second+" in "+iter+".values()", &Tuple{Elems: []Type{n.Args[0], n.Args[1]}}
		case two:
			form, elem = "for ("+first+", "+second+") in "+iter, &Tuple{Elems: []Type{n.Args[0], n.Args[1]}}
		default:
			form, elem = "for "+first+" in "+iter+".keys()", n.Args[0]
		}
	case kind == "Chan" && !two:
		form, elem = "for "+first+" in "+iter, n.Args[0]
	case kind == "Int" && !two:
		form, elem = "for "+first+" in 0.."+iter, intT
	case kind == "Str" && two && first == "_":
		form, elem = "for "+second+" in "+iter+".runes()", &Tuple{Elems: []Type{intT, c.named("Rune")}}
	}
	if form == "" {
		note(d, token.Span{}, "Xo iterates with `for x in xs`, `for (i, x) in xs.indexed()`, `for (k, v) in m`, and `for i in 0..n` (core 3.5)")
		return badType
	}
	note(d, token.Span{}, "for this `%s`, write `%s`", deep(it), form)
	fix(d, "write `"+form+"`", replaceEdit(header, form+" "))
	return elem
}

// elidedEffectNote explains XO0701 when the missing effect is an effect
// argument left out of a parameter type (`Cache[Int, Item]` for
// `Cache[K, V, fx: Effects]`): inside the function it is an abstract
// effect variable that a closure's own `uses` clause cannot name. The fix
// removes the closure's clause so its effects are inferred; writing the
// effect argument is the other way (decision 0087).
func (c *checker) elidedEffectNote(d *diag.Diagnostic, sub, sup *flatEff, ob effObligation) {
	for p := range sub.params {
		if _, ok := sup.params[p]; ok || !p.Elided || p.ElidedAt.File == "" {
			continue
		}
		at := diag.FromSpan(p.ElidedAt)
		at.EndLine, at.EndCol = 0, 0
		d.Notes = append(d.Notes, diag.NoteItem{
			Message: fmt.Sprintf("`%s` is the effect argument left out of `%s` here; an omitted effect argument stays an abstract effect variable of the function (core 6.2), as Rust and TypeScript generics would infer it, so a `uses` list cannot name it: write the effect argument, `%s[..., net.dial]` naming the effects of the value passed in, or let the closure's effects be inferred", p.Name, p.ElidedText, typeName(p.ElidedText)),
			Span:    &at,
		})
		if ob.usesClause.File != "" {
			fix(d, "remove the closure's `uses` clause so its effects are inferred", replaceEdit(ob.usesClause, ""))
		}
		return
	}
}

// usesClauseSpan is the span of ` uses ...` in a closure literal, from
// the blank before `uses` to the end of the effect list.
func (c *checker) usesClauseSpan(x *ast.FnLit) token.Span {
	if len(x.Uses) == 0 || c.file == nil {
		return token.Span{}
	}
	src := c.file.Src
	first, last := x.Uses[0].Sp, x.Uses[len(x.Uses)-1].Sp
	i := first.Start.Offset
	for i > 0 && (src[i-1] == ' ' || src[i-1] == '(') {
		i--
	}
	if i < 4 || string(src[i-4:i]) != "uses" {
		return token.Span{}
	}
	i -= 4
	for i > 0 && src[i-1] == ' ' {
		i--
	}
	end := last.End.Offset
	j := end
	for j < len(src) && src[j] == ' ' {
		j++
	}
	if j < len(src) && src[j] == ')' {
		end = j + 1
	}
	if first.Start.Line != last.End.Line {
		return token.Span{}
	}
	line := first.Start.Line
	start := token.Pos{Line: line, Col: first.Start.Col - (first.Start.Offset - i), Offset: i}
	stop := token.Pos{Line: line, Col: last.End.Col + (end - last.End.Offset), Offset: end}
	return token.Span{File: x.Sp.File, Start: start, End: stop}
}

// habitAssignDecl handles Python's declaration by assignment, `t = 0` for
// a name that is not declared: it reports XO0601 with the Xo form and
// declares the name, so its later uses resolve.
func (c *checker) habitAssignDecl(x *ast.AssignStmt) bool {
	id, ok := x.Target.(*ast.Ident)
	if !ok || x.Op != token.Assign || id.Name == "_" || c.lookupLocal(id.Name) != nil ||
		c.lookupModuleObj(c.mod, id.Name) != nil || c.file.Imports[id.Name] != nil || c.ctx == nil {
		return false
	}
	switch id.Name {
	case "true", "false", "target", "self", "os", "expect", "testing":
		return false
	}
	t := c.infer(x.Value, nil)
	d := c.errorf(diag.UnresolvedName, id.Sp, "name `%s` not found", id.Name)
	habitNote(d, "`%s = v` declares a variable in Python; Xo declares a name before assigning it: `var %s = v`, or `let %s = v` when it never changes (core 3.1)", id.Name, id.Name, id.Name)
	fix(d, "declare with `var`", insertEdit(x.Sp.File, x.Sp.Start, "var "))
	decl := &ast.VarStmt{Base: x.Base, Name: id, Value: x.Value}
	v := &VarObj{Name: id.Name, Type: t, Mutable: true, Kind: VLocal, Span: id.Sp, isVarStmt: true, decl: decl}
	c.declareVar(v)
	c.identObj[id] = v
	return true
}

// callMethodQuiet checks a recovered method call whose diagnostic was
// already reported.
func (c *checker) callMethodQuiet(bt Type, call *ast.CallExpr, hint Type) Type {
	sel := call.Fun.(*ast.SelectorExpr)
	return c.callMethod(bt, sel, nil, call, hint)
}

// habitField handles a field access that does not resolve: `xs.length`
// (TypeScript), `xs.count` (Swift), `s.isEmpty` (Swift).
func (c *checker) habitField(bt Type, x *ast.SelectorExpr, place bool) (Type, bool) {
	if place {
		return nil, false
	}
	r := resolve(bt)
	name := x.Sel.Name
	xo := map[string]string{"length": "len", "count": "len", "size": "len", "isEmpty": "is_empty"}[name]
	if xo == "" {
		return nil, false
	}
	m, _ := c.findMethodAny(r, xo)
	if m == nil {
		return nil, false
	}
	d := c.errorf(diag.UnresolvedName, x.Sel.Sp, "`%s` has no field `%s`", deep(bt), name)
	habitNote(d, "`.%s` is a property in TypeScript, Swift, and Kotlin; in Xo it is a method call, `.%s()` (spec/std.md 2)", name, xo)
	fix(d, "write `."+xo+"()`", replaceEdit(x.Sel.Sp, xo+"()"))
	call := &ast.CallExpr{Base: x.Base, Fun: &ast.SelectorExpr{Base: x.Base, X: x.X, Sel: &ast.Ident{Base: x.Sel.Base, Name: xo}}}
	return c.callMethod(bt, call.Fun.(*ast.SelectorExpr), nil, call, nil), true
}

// typeText renders a type as written, for notes.
func typeText(t ast.Type) string {
	switch x := t.(type) {
	case *ast.NamedType:
		var parts []string
		for _, p := range x.Path {
			parts = append(parts, p.Name)
		}
		s := strings.Join(parts, ".")
		if len(x.Args) > 0 {
			var args []string
			for _, a := range x.Args {
				args = append(args, typeText(a))
			}
			s += "[" + strings.Join(args, ", ") + "]"
		}
		return s
	case *ast.OptionalType:
		return typeText(x.Elem) + "?"
	case *ast.TupleType:
		var elems []string
		for _, e := range x.Elems {
			elems = append(elems, typeText(e))
		}
		return "(" + strings.Join(elems, ", ") + ")"
	}
	return "..."
}

// typeName is the name part of a rendered type, without arguments.
func typeName(s string) string {
	if i := strings.IndexByte(s, '['); i >= 0 {
		return s[:i]
	}
	return s
}
