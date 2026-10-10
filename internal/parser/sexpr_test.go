package parser

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
)

// sexpr renders a node as a compact S-expression for table driven tests.
func sexpr(n any) string {
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }
	list := func(prefix string, items []string) string {
		if len(items) == 0 {
			return "(" + prefix + ")"
		}
		return "(" + prefix + " " + strings.Join(items, " ") + ")"
	}
	opt := func(n any) string {
		if isNil(n) {
			return "_"
		}
		return sexpr(n)
	}
	switch x := n.(type) {
	case nil:
		return "_"
	case *ast.File:
		var items []string
		for _, u := range x.Uses {
			items = append(items, sexpr(u))
		}
		for _, d := range x.Decls {
			items = append(items, sexpr(d))
		}
		return strings.Join(items, "\n")
	case *ast.UseDecl:
		if x.C {
			s := fmt.Sprintf("(use c %q", x.Path)
			for _, l := range x.Links {
				s += fmt.Sprintf(" link %q", l)
			}
			if x.Alias != nil {
				s += " as " + x.Alias.Name
			}
			if x.Uses != nil {
				s += " uses"
				for _, e := range x.Uses {
					s += " " + e.Name.Name
					if e.Sub != nil {
						s += "." + e.Sub.Name
					}
				}
			}
			return s + ")"
		}
		if x.Go {
			if x.Alias != nil {
				return fmt.Sprintf("(use go %q as %s)", x.Path, x.Alias.Name)
			}
			return fmt.Sprintf("(use go %q)", x.Path)
		}
		if x.Alias != nil {
			return fmt.Sprintf("(use %s as %s)", x.Path, x.Alias.Name)
		}
		return fmt.Sprintf("(use %s)", x.Path)
	case *ast.FnDecl:
		var items []string
		if x.Pub {
			items = append(items, "pub")
		}
		if x.Recv != nil {
			v := ""
			if x.Recv.Var {
				v = "var "
			}
			items = append(items, fmt.Sprintf("(recv %s%s %s)", v, x.Recv.Name.Name, sexpr(x.Recv.Type)))
		}
		items = append(items, x.Name.Name)
		if len(x.TypeParams) > 0 {
			items = append(items, typeParams(x.TypeParams))
		}
		items = append(items, params(x.Params))
		if x.Result != nil {
			items = append(items, "-> "+sexpr(x.Result))
		}
		if x.Err != nil {
			items = append(items, "! "+opt(x.Err.Type))
		}
		for _, c := range x.Clauses {
			items = append(items, sexpr(c))
		}
		if x.Body != nil {
			items = append(items, sexpr(x.Body))
		}
		return list("fn", items)
	case *ast.UsesClause:
		return list("uses", effects(x.Effects))
	case *ast.RequiresClause:
		s := "requires"
		if x.Debug {
			s += "(debug)"
		}
		if x.Else != nil {
			return fmt.Sprintf("(%s %s else %s)", s, sexpr(x.Cond), sexpr(x.Else))
		}
		return fmt.Sprintf("(%s %s)", s, sexpr(x.Cond))
	case *ast.EnsuresClause:
		return fmt.Sprintf("(ensures %s)", sexpr(x.Cond))
	case *ast.StructDecl:
		var items []string
		if x.Pub {
			items = append(items, "pub")
		}
		items = append(items, x.Name.Name)
		if len(x.TypeParams) > 0 {
			items = append(items, typeParams(x.TypeParams))
		}
		for _, f := range x.Fields {
			s := f.Name.Name + ": " + sexpr(f.Type)
			if f.Pub {
				s = "pub " + s
			}
			if f.Default != nil {
				s += " = " + sexpr(f.Default)
			}
			items = append(items, "["+s+"]")
		}
		if x.Invariant != nil {
			items = append(items, "(invariant "+sexpr(x.Invariant)+")")
		}
		if len(x.Derive) > 0 {
			items = append(items, list("derive", idents(x.Derive)))
		}
		return list("struct", items)
	case *ast.EnumDecl:
		items := []string{x.Name.Name}
		if len(x.TypeParams) > 0 {
			items = append(items, typeParams(x.TypeParams))
		}
		for _, v := range x.Variants {
			if v.Fields == nil {
				items = append(items, v.Name.Name)
				continue
			}
			var fs []string
			for _, f := range v.Fields {
				if f.From {
					fs = append(fs, "from "+sexpr(f.Type))
				} else {
					fs = append(fs, f.Name.Name+": "+sexpr(f.Type))
				}
			}
			items = append(items, v.Name.Name+"("+strings.Join(fs, ", ")+")")
		}
		if len(x.Derive) > 0 {
			items = append(items, list("derive", idents(x.Derive)))
		}
		return list("enum", items)
	case *ast.InterfaceDecl:
		items := []string{x.Name.Name}
		for _, m := range x.Methods {
			items = append(items, sexpr(m))
		}
		return list("interface", items)
	case *ast.TypeDecl:
		if x.Newtype {
			return fmt.Sprintf("(newtype %s %s)", x.Name.Name, sexpr(x.Type))
		}
		return fmt.Sprintf("(alias %s %s)", x.Name.Name, sexpr(x.Type))
	case *ast.ConstDecl:
		if x.Type != nil {
			return fmt.Sprintf("(const %s %s %s)", x.Name.Name, sexpr(x.Type), sexpr(x.Value))
		}
		return fmt.Sprintf("(const %s %s)", x.Name.Name, sexpr(x.Value))
	case *ast.TestDecl:
		items := []string{sexpr(x.Name)}
		if len(x.For) > 0 {
			items = append(items, "for "+params(x.For))
		}
		if x.Seed != nil {
			items = append(items, "seed "+x.Seed.Raw)
		}
		if len(x.Uses) > 0 {
			items = append(items, list("uses", effects(x.Uses)))
		}
		items = append(items, sexpr(x.Body))
		return list("test", items)
	case *ast.BadDecl:
		return "(bad-decl)"

	// Types
	case *ast.NamedType:
		s := strings.Join(idents(x.Path), ".")
		if len(x.Args) > 0 {
			var as []string
			for _, a := range x.Args {
				as = append(as, sexpr(a))
			}
			s += "[" + strings.Join(as, ", ") + "]"
		}
		return s
	case *ast.OptionalType:
		return sexpr(x.Elem) + "?"
	case *ast.TupleType:
		var es []string
		for _, e := range x.Elems {
			es = append(es, sexpr(e))
		}
		return "(" + strings.Join(es, ", ") + ")"
	case *ast.FnType:
		var ps []string
		for _, p := range x.Params {
			s := sexpr(p.Type)
			if p.Var {
				s = "var " + s
			}
			ps = append(ps, s)
		}
		s := "fn(" + strings.Join(ps, ", ") + ")"
		if x.Result != nil {
			s += " -> " + sexpr(x.Result)
		}
		if x.Err != nil {
			s += " ! " + opt(x.Err.Type)
		}
		if len(x.Uses) > 0 {
			s += " uses (" + strings.Join(effects(x.Uses), ", ") + ")"
		}
		return "<" + s + ">"
	case *ast.BadType:
		return "(bad-type)"

	// Statements
	case *ast.LetStmt:
		if x.Type != nil {
			return fmt.Sprintf("(let %s %s %s)", sexpr(x.Pattern), sexpr(x.Type), sexpr(x.Value))
		}
		return fmt.Sprintf("(let %s %s)", sexpr(x.Pattern), sexpr(x.Value))
	case *ast.VarStmt:
		if x.Type != nil {
			return fmt.Sprintf("(var %s %s %s)", x.Name.Name, sexpr(x.Type), sexpr(x.Value))
		}
		return fmt.Sprintf("(var %s %s)", x.Name.Name, sexpr(x.Value))
	case *ast.AssignStmt:
		return fmt.Sprintf("(%s %s %s)", x.Op, sexpr(x.Target), sexpr(x.Value))
	case *ast.DeferStmt:
		return fmt.Sprintf("(defer %s)", sexpr(x.Body))
	case *ast.ReturnStmt:
		if x.Value == nil {
			return "(return)"
		}
		return fmt.Sprintf("(return %s)", sexpr(x.Value))
	case *ast.BreakStmt:
		if x.Label != nil {
			return "(break " + x.Label.Name + ")"
		}
		return "(break)"
	case *ast.ContinueStmt:
		if x.Label != nil {
			return "(continue " + x.Label.Name + ")"
		}
		return "(continue)"
	case *ast.ForStmt:
		l := ""
		if x.Label != nil {
			l = x.Label.Name + ": "
		}
		return fmt.Sprintf("(%sfor %s %s %s)", l, sexpr(x.Pattern), sexpr(x.Iter), sexpr(x.Body))
	case *ast.WhileStmt:
		l := ""
		if x.Label != nil {
			l = x.Label.Name + ": "
		}
		return fmt.Sprintf("(%swhile %s %s)", l, opt(x.Cond), sexpr(x.Body))
	case *ast.ExprStmt:
		return sexpr(x.X)
	case *ast.BadStmt:
		return "(bad-stmt)"

	// Expressions
	case *ast.Ident:
		return x.Name
	case *ast.IntLit:
		return x.Raw
	case *ast.FloatLit:
		return x.Raw
	case *ast.DurationLit:
		return x.Raw
	case *ast.RuneLit:
		return x.Raw
	case *ast.StringLit:
		if len(x.Parts) == 1 && x.Parts[0].Expr == nil {
			return strconv.Quote(x.Parts[0].Text)
		}
		var ps []string
		for _, p := range x.Parts {
			if p.Expr != nil {
				ps = append(ps, "${"+sexpr(p.Expr)+"}")
			} else {
				ps = append(ps, strconv.Quote(p.Text))
			}
		}
		return list("str", ps)
	case *ast.BytesLit:
		return "b" + strconv.Quote(x.Value)
	case *ast.HoleExpr:
		return "???"
	case *ast.UnitExpr:
		return "()"
	case *ast.ParenExpr:
		return "(paren " + sexpr(x.X) + ")"
	case *ast.TupleExpr:
		return list("tuple", exprs(x.Elems))
	case *ast.ListLit:
		return "[" + strings.Join(exprs(x.Elems), " ") + "]"
	case *ast.MapLit:
		var es []string
		for _, e := range x.Entries {
			es = append(es, "("+sexpr(e.Key)+" "+sexpr(e.Value)+")")
		}
		return list("map", es)
	case *ast.SetLit:
		return "#{" + strings.Join(exprs(x.Elems), " ") + "}"
	case *ast.StructLit:
		return list("struct "+sexpr(x.Type), fieldInits(x.Fields))
	case *ast.UnaryExpr:
		return fmt.Sprintf("(%s %s)", x.Op, sexpr(x.X))
	case *ast.BinaryExpr:
		return fmt.Sprintf("(%s %s %s)", x.Op, sexpr(x.X), sexpr(x.Y))
	case *ast.IsExpr:
		return fmt.Sprintf("(is %s %s)", sexpr(x.X), sexpr(x.Pattern))
	case *ast.RangeExpr:
		op := ".."
		if x.Inclusive {
			op = "..="
		}
		return fmt.Sprintf("(%s %s %s)", op, opt(x.Lo), opt(x.Hi))
	case *ast.SelectorExpr:
		return fmt.Sprintf("(. %s %s)", sexpr(x.X), x.Sel.Name)
	case *ast.CallExpr:
		var as []string
		for _, a := range x.Args {
			if a.Name != nil {
				as = append(as, a.Name.Name+": "+sexpr(a.Value))
			} else {
				as = append(as, sexpr(a.Value))
			}
		}
		return list("call "+sexpr(x.Fun), as)
	case *ast.TypeArgsExpr:
		var ts []string
		for _, t := range x.Types {
			ts = append(ts, sexpr(t))
		}
		return fmt.Sprintf("(targs %s %s)", sexpr(x.X), strings.Join(ts, " "))
	case *ast.IndexExpr:
		return fmt.Sprintf("(index %s %s)", sexpr(x.X), sexpr(x.Index))
	case *ast.TryExpr:
		return fmt.Sprintf("(? %s)", sexpr(x.X))
	case *ast.WithExpr:
		return list("with "+sexpr(x.X), fieldInits(x.Fields))
	case *ast.IfExpr:
		head := sexpr(x.Cond)
		kw := "if"
		if x.Pattern != nil {
			kw = "iflet"
			head = sexpr(x.Pattern) + " " + sexpr(x.Value)
		}
		if x.Else != nil {
			return fmt.Sprintf("(%s %s %s %s)", kw, head, sexpr(x.Then), sexpr(x.Else))
		}
		return fmt.Sprintf("(%s %s %s)", kw, head, sexpr(x.Then))
	case *ast.MatchExpr:
		var arms []string
		for _, a := range x.Arms {
			if a.Guard != nil {
				arms = append(arms, fmt.Sprintf("(%s if %s => %s)", sexpr(a.Pattern), sexpr(a.Guard), sexpr(a.Body)))
			} else {
				arms = append(arms, fmt.Sprintf("(%s => %s)", sexpr(a.Pattern), sexpr(a.Body)))
			}
		}
		return list("match "+sexpr(x.Subject), arms)
	case *ast.ScopeExpr:
		return fmt.Sprintf("(scope %s %s)", x.Name.Name, sexpr(x.Body))
	case *ast.SelectExpr:
		var arms []string
		for _, a := range x.Arms {
			switch {
			case a.After != nil:
				arms = append(arms, fmt.Sprintf("(after %s => %s)", sexpr(a.After), sexpr(a.Body)))
			case a.Bind != nil:
				arms = append(arms, fmt.Sprintf("(%s = %s => %s)", a.Bind.Name, sexpr(a.Op), sexpr(a.Body)))
			default:
				arms = append(arms, fmt.Sprintf("(%s => %s)", sexpr(a.Op), sexpr(a.Body)))
			}
		}
		return list("select", arms)
	case *ast.Block:
		var ss []string
		for _, s := range x.Stmts {
			ss = append(ss, sexpr(s))
		}
		return "{" + strings.Join(ss, "; ") + "}"
	case *ast.FnLit:
		items := []string{params(x.Params)}
		if x.Result != nil {
			items = append(items, "-> "+sexpr(x.Result))
		}
		if x.Err != nil {
			items = append(items, "! "+opt(x.Err.Type))
		}
		if len(x.Uses) > 0 {
			items = append(items, list("uses", effects(x.Uses)))
		}
		items = append(items, sexpr(x.Body))
		return list("fn", items)
	case *ast.WhileExpr:
		return "(loop " + sexpr(x.Body) + ")"
	case *ast.BadExpr:
		return "(bad)"

	// Patterns
	case *ast.WildcardPat:
		return "_"
	case *ast.BindPat:
		return x.Name.Name
	case *ast.LitPat:
		return sexpr(x.Value)
	case *ast.RangePat:
		op := ".."
		if x.Inclusive {
			op = "..="
		}
		return fmt.Sprintf("(%s %s %s)", op, sexpr(x.Lo), sexpr(x.Hi))
	case *ast.OrPat:
		var as []string
		for _, a := range x.Alts {
			as = append(as, sexpr(a))
		}
		return list("|", as)
	case *ast.TuplePat:
		var es []string
		for _, e := range x.Elems {
			es = append(es, sexpr(e))
		}
		return list("tuple", es)
	case *ast.VariantPat:
		s := strings.Join(idents(x.Path), ".")
		if x.Parens {
			s += "(" + strings.Join(patFields(x.Fields, x.Rest), ", ") + ")"
		}
		return s
	case *ast.StructPat:
		return strings.Join(idents(x.Path), ".") + "{" + strings.Join(patFields(x.Fields, x.Rest), ", ") + "}"
	case *ast.ListPat:
		var es []string
		for i, e := range x.Elems {
			if i == x.Rest {
				es = append(es, "..")
			}
			es = append(es, sexpr(e))
		}
		if x.Rest == len(x.Elems) {
			es = append(es, "..")
		}
		return "[" + strings.Join(es, ", ") + "]"
	case *ast.BadPat:
		return "(bad-pat)"
	}
	w("<?%T>", n)
	return b.String()
}

func isNil(n any) bool {
	if n == nil {
		return true
	}
	v := reflect.ValueOf(n)
	return v.Kind() == reflect.Pointer && v.IsNil()
}

func exprs(es []ast.Expr) []string {
	var out []string
	for _, e := range es {
		out = append(out, sexpr(e))
	}
	return out
}

func idents(ids []*ast.Ident) []string {
	var out []string
	for _, id := range ids {
		out = append(out, id.Name)
	}
	return out
}

func effects(es []*ast.Effect) []string {
	var out []string
	for _, e := range es {
		out = append(out, effectText(e))
	}
	return out
}

func typeParams(tps []*ast.TypeParam) string {
	var out []string
	for _, tp := range tps {
		s := tp.Name.Name
		if tp.Effects {
			s += ": Effects"
		} else if tp.Constraint != nil {
			s += ": " + sexpr(tp.Constraint)
		}
		out = append(out, s)
	}
	return "[" + strings.Join(out, ", ") + "]"
}

func params(ps []*ast.Param) string {
	var out []string
	for _, p := range ps {
		s := p.Name.Name
		if p.Var {
			s = "var " + s
		}
		if p.Type != nil {
			s += ": " + sexpr(p.Type)
		}
		if p.Default != nil {
			s += " = " + sexpr(p.Default)
		}
		out = append(out, s)
	}
	return "(" + strings.Join(out, ", ") + ")"
}

func fieldInits(fs []*ast.FieldInit) []string {
	var out []string
	for _, f := range fs {
		out = append(out, "("+f.Name.Name+" "+sexpr(f.Value)+")")
	}
	return out
}

func patFields(fs []*ast.PatField, rest bool) []string {
	var out []string
	for _, f := range fs {
		if f.Name != nil {
			out = append(out, f.Name.Name+": "+sexpr(f.Pattern))
		} else {
			out = append(out, sexpr(f.Pattern))
		}
	}
	if rest {
		out = append(out, "..")
	}
	return out
}
