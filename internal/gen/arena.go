package gen

// Request scoped arenas (Options.Arenas; internal/build sets it unless
// `--no-arenas`, decision 0066). The runtime side and the soundness
// argument are in std/rt/arena.go; this file is the escape analysis that
// decides what may live in an arena. All of it is
// intraprocedural and syntactic over checked code, and it relies on Xo's
// value semantics: a value can only reach shared state by being copied
// into it, and only through a binding, an argument, a capture, or a
// result, so a value that is never bound, passed, captured, or returned
// cannot escape.
//
//   - Handler confinement (confinesReq): a closure literal passed to
//     `r.handle(pattern, fn(req) {...})` whose parameter is used only in
//     the forms listed in std/rt/arena.go, never inside a nested closure.
//     The route is then marked with rt.Http_Router_arenaLast and its
//     request may be arena memory; any other handler gets a heap copy.
//   - Arena responses (markRespCalls): `http.json(...)` and `http.text(...)`
//     calls whose value is the handler literal's result: the tail
//     expression of its body (through if, match, and block tails) or the
//     value of a `return` in it (not in a nested closure). Their value is
//     returned to the runtime, which writes it before the arena is reset.
//   - Expression temporaries (noteArenaRecv, arenaProducer): `m.values()`,
//     `m.keys()`, `m.entries()`, `s.to_list()` whose List is only the
//     receiver of a consumer in arenaConsumers or the iterable of a `for`
//     loop. The List lives in a compiler temporary that no Xo code can
//     name, and every consumer copies elements out and returns a fresh
//     value.
//
// With Options.Arenas off nothing here runs and the generated code is
// unchanged.

import (
	"fmt"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

// arenasOn reports whether arena lowering is on for this package.
func (f *fnGen) arenasOn() bool { return f.g.opts.Arenas && !f.pg.inRT }

// reqMethods are the Request methods whose results never share memory
// with the request (std/rt/arena.go): they return Int, heap strings, or
// freshly decoded values.
var reqMethods = map[string]bool{"json": true, "param": true, "param_int": true, "query": true, "header": true}

// reqFields are the Str fields of a Request that net/http allocates (never
// arena memory); reqURLFields the same for `req.url`.
var (
	reqFields    = map[string]bool{"method": true, "path": true, "remote_ip": true}
	reqURLFields = map[string]bool{"scheme": true, "host": true, "port": true, "path": true}
)

// confinesReq reports whether handler literal lit uses its request
// parameter only in forms that cannot let the request (its tables and
// body) escape.
func (f *fnGen) confinesReq(lit *ast.FnLit) bool {
	ps := f.info.ClosureParams(lit)
	if len(ps) != 1 {
		return false
	}
	req := ps[0]
	reqIdent := func(e ast.Expr) *ast.Ident {
		id, ok := unparen(e).(*ast.Ident)
		if ok && f.info.VarOf(id) == req {
			return id
		}
		return nil
	}
	allowed := map[*ast.Ident]bool{}
	inspectNode(lit.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			ci := f.info.CallOf(x)
			if ci != nil && ci.Fn != nil && ci.Recv != nil && reqMethods[ci.Fn.Name] && isStdMethod(ci.Fn, "http", "Request", ci.Fn.Name) {
				if id := reqIdent(ci.Recv); id != nil {
					allowed[id] = true
				}
			}
		case *ast.SelectorExpr:
			if id := reqIdent(x.X); id != nil && reqFields[x.Sel.Name] {
				allowed[id] = true
			}
			if in, ok := unparen(x.X).(*ast.SelectorExpr); ok && in.Sel.Name == "url" && reqURLFields[x.Sel.Name] {
				if id := reqIdent(in.X); id != nil {
					allowed[id] = true
				}
			}
		}
		return true
	})
	ok := true
	inspectNode(lit.Body, func(n ast.Node) bool {
		if !ok {
			return false
		}
		switch x := n.(type) {
		case *ast.FnLit:
			// a nested closure may outlive the request (a background
			// task): it must not mention the request at all
			inspectNode(x.Body, func(m ast.Node) bool {
				if id, isID := m.(*ast.Ident); isID && f.info.VarOf(id) == req {
					ok = false
				}
				return ok
			})
			return false
		case *ast.Ident:
			if f.info.VarOf(x) == req && !allowed[x] {
				ok = false
			}
		}
		return ok
	})
	return ok
}

// isRespCall reports whether ci calls `http.json` or `http.text`.
func isRespCall(ci *check.Call) bool {
	if ci == nil || ci.Fn == nil || ci.Fn.Recv != nil || !ci.Fn.Module.IsStd || ci.Fn.Module.Path != "std/http" {
		return false
	}
	return ci.Fn.Name == "json" || ci.Fn.Name == "text"
}

// markRespCalls records the response calls whose value is the result of
// handler literal lit.
func (f *fnGen) markRespCalls(lit *ast.FnLit) {
	var tail func(e ast.Expr)
	tailBlock := func(b *ast.Block) {
		if b == nil || len(b.Stmts) == 0 {
			return
		}
		if es, ok := b.Stmts[len(b.Stmts)-1].(*ast.ExprStmt); ok {
			tail(es.X)
		}
	}
	tail = func(e ast.Expr) {
		switch x := unparen(e).(type) {
		case *ast.CallExpr:
			if isRespCall(f.info.CallOf(x)) {
				f.arenaResp[x] = true
			}
		case *ast.Block:
			tailBlock(x)
		case *ast.IfExpr:
			tailBlock(x.Then)
			if x.Else != nil {
				tail(x.Else)
			}
		case *ast.MatchExpr:
			for _, arm := range x.Arms {
				if es, ok := arm.Body.(*ast.ExprStmt); ok {
					tail(es.X)
				}
			}
		}
	}
	tailBlock(lit.Body)
	inspectNode(lit.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FnLit:
			return false // its own result
		case *ast.ReturnStmt:
			if x.Value != nil {
				tail(x.Value)
			}
		}
		return true
	})
}

// handlerLit returns the closure literal passed as the handler of an
// `r.handle(pattern, h)` call on a local router, or nil.
func (f *fnGen) handlerLit(x *ast.CallExpr, ci *check.Call) *ast.FnLit {
	if !isStdMethod(ci.Fn, "http", "Router", "handle") || ci.Recv == nil {
		return nil
	}
	if _, ok := unparen(ci.Recv).(*ast.Ident); !ok {
		return nil
	}
	exprs := argExprs(ci.Fn.Params, x)
	if len(exprs) != 2 || exprs[1] == nil {
		return nil
	}
	lit, _ := unparen(exprs[1]).(*ast.FnLit)
	return lit
}

// arenaConsumers are the List methods that neither return nor keep their
// receiver (or a slice of it): each copies elements out and returns a
// fresh value (std/rt/list.go; the inlined forms in mono.go likewise).
var arenaConsumers = map[string]bool{
	"any": true, "all": true, "find": true, "fold": true, "map": true, "filter": true,
	"sort_by": true, "sort": true, "len": true, "is_empty": true, "contains": true,
	"index_of": true, "first": true, "last": true, "get": true, "join": true,
	"reverse": true, "to_set": true, "indexed": true,
}

// arenaProducerFunc names the runtime function that builds the temporary
// List of producer fo in the arena, or "".
func arenaProducerFunc(fo *check.FuncObj) string {
	switch {
	case isStdMethod(fo, "prelude", "Map", "values"):
		return "Map_values_arena"
	case isStdMethod(fo, "prelude", "Map", "keys"):
		return "Map_keys_arena"
	case isStdMethod(fo, "prelude", "Map", "entries"):
		return "Map_entries_arena"
	case isStdMethod(fo, "prelude", "Set", "to_list"):
		return "Set_to_list_arena"
	}
	return ""
}

// noteArenaTemp marks e as an arena temporary if it is a producer call.
func (f *fnGen) noteArenaTemp(e ast.Expr) {
	c, ok := unparen(e).(*ast.CallExpr)
	if !ok {
		return
	}
	if pci := f.info.CallOf(c); pci != nil && pci.Fn != nil && arenaProducerFunc(pci.Fn) != "" {
		if f.arenaTemps == nil {
			f.arenaTemps = map[*ast.CallExpr]bool{}
		}
		f.arenaTemps[c] = true
	}
}

// noteArenaCall runs before a std call is generated: it marks a producer
// receiver of a consumer, and the response calls of a handler literal.
func (f *fnGen) noteArenaCall(x *ast.CallExpr, ci *check.Call) {
	fo := ci.Fn
	if ci.Recv != nil && fo.Recv != nil && fo.Recv.Module.IsPrelude && fo.Recv.Name == "List" && arenaConsumers[fo.Name] {
		f.noteArenaTemp(ci.Recv)
	}
	if lit := f.handlerLit(x, ci); lit != nil {
		if f.arenaResp == nil {
			f.arenaResp = map[*ast.CallExpr]bool{}
		}
		f.markRespCalls(lit)
	}
}

// arenaProducer generates a producer call marked as an arena temporary.
func (f *fnGen) arenaProducer(x *ast.CallExpr, ci *check.Call) (string, bool) {
	if !f.arenaTemps[x] || ci.Recv == nil {
		return "", false
	}
	name := arenaProducerFunc(ci.Fn)
	n := f.g.named(f.typeOf(ci.Recv))
	if name == "" || n == nil || len(n.Args) == 0 {
		return "", false
	}
	targs := make([]string, len(n.Args))
	for i, a := range n.Args {
		targs[i] = f.goType(a)
	}
	return fmt.Sprintf("%s%s[%s](tk, %s)", f.rt(), name, strings.Join(targs, ", "), f.val(ci.Recv)), true
}

// arenaRespCode renames a generated `http.json`/`http.text` call to its
// arena form when x is a marked response call.
func (f *fnGen) arenaRespCode(x *ast.CallExpr, code string) string {
	if !f.arenaResp[x] {
		return code
	}
	r := f.rt()
	for _, name := range []string{"Http_json", "Http_text"} {
		if strings.HasPrefix(code, r+name+"[") || strings.HasPrefix(code, r+name+"(") {
			return r + name + "_arena" + code[len(r+name):]
		}
	}
	return code
}

// arenaRoute emits the mark of a confined handler after an `r.handle` call.
func (f *fnGen) arenaRoute(x *ast.CallExpr, ci *check.Call, recv string) bool {
	lit := f.handlerLit(x, ci)
	if lit == nil || !f.confinesReq(lit) {
		return false
	}
	f.line("%sHttp_Router_arenaLast(&%s)", f.rt(), recv)
	return true
}
