package ast

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/xo-lang/xo/internal/token"
)

// Text renders a syntax tree in the canonical text form shared with the
// self hosted parser in selfhost/parser (its dump.xo); `xo astdump` prints
// it and internal/selfhost compares the two byte for byte (decision 0127).
//
// One node per line, children indented two spaces deeper than their
// parent:
//
//	File <span> name="<file>"
//	  <field>: <Kind> <span> <attrs>     a node held in a field
//	  <field>[<i>]: <Kind> <span> ...    element i of a list field
//	  <field>: []                        an empty list that is not nil
//
// Kind is the Go type name of the node. A span is
// line:col:offset-line:col:offset. Attributes are the scalar fields in
// declaration order, snake_case: strings as name="..." (always, quoted
// like the token dump: printable ASCII stays, `"` and `\` are escaped,
// every other byte is \xHH), true booleans as a bare name, integers as
// name=N, token kinds by their Go constant name (op=Plus), a rune as its
// code point, a string kind as plain, triple, or raw, and a list of
// strings as name=["a","b"]. Child fields (nodes, lists of nodes) follow
// in declaration order; nil ones are omitted. StringPart has no span.
func Text(f *File) string {
	var b strings.Builder
	textNode(&b, "", 0, reflect.ValueOf(f))
	return b.String()
}

var (
	stringKindType = reflect.TypeOf(StringKind(0))
	runeType       = reflect.TypeOf(rune(0))
	baseType       = reflect.TypeOf(Base{})
)

// textNode prints the node v (a pointer to a node struct, an interface
// holding one, or a StringPart) under label at depth.
func textNode(b *strings.Builder, label string, depth int, v reflect.Value) {
	for v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	t := v.Type()
	b.WriteString(strings.Repeat("  ", depth))
	if label != "" {
		b.WriteString(label + ": ")
	}
	b.WriteString(t.Name())
	type child struct {
		name string
		v    reflect.Value
	}
	var kids []child
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fv := v.Field(i)
		if f.Anonymous && f.Type == baseType {
			b.WriteString(" " + textSpan(fv.Field(0).Interface().(token.Span)))
			continue
		}
		name := jsonName(f.Name)
		switch {
		case f.Type == kindType:
			b.WriteString(" " + name + "=" + kindName(token.Kind(fv.Int())))
		case f.Type == stringKindType:
			b.WriteString(" " + name + "=" + [...]string{"plain", "triple", "raw"}[fv.Int()])
		case f.Type == runeType:
			b.WriteString(" " + name + "=" + strconv.FormatInt(fv.Int(), 10))
		case fv.Kind() == reflect.Bool:
			if fv.Bool() {
				b.WriteString(" " + name)
			}
		case fv.Kind() == reflect.String:
			b.WriteString(" " + name + "=" + textQuote(fv.String()))
		case fv.Kind() == reflect.Int:
			b.WriteString(" " + name + "=" + strconv.FormatInt(fv.Int(), 10))
		case fv.Kind() == reflect.Slice && f.Type.Elem().Kind() == reflect.String:
			if fv.IsNil() {
				continue
			}
			var q []string
			for j := 0; j < fv.Len(); j++ {
				q = append(q, textQuote(fv.Index(j).String()))
			}
			b.WriteString(" " + name + "=[" + strings.Join(q, ",") + "]")
		default:
			kids = append(kids, child{name, fv})
		}
	}
	b.WriteByte('\n')
	for _, k := range kids {
		switch k.v.Kind() {
		case reflect.Slice:
			if k.v.IsNil() {
				continue
			}
			if k.v.Len() == 0 {
				fmt.Fprintf(b, "%s%s: []\n", strings.Repeat("  ", depth+1), k.name)
				continue
			}
			for j := 0; j < k.v.Len(); j++ {
				textNode(b, fmt.Sprintf("%s[%d]", k.name, j), depth+1, k.v.Index(j))
			}
		case reflect.Pointer, reflect.Interface:
			if k.v.IsNil() {
				continue
			}
			textNode(b, k.name, depth+1, k.v)
		default:
			panic(fmt.Sprintf("ast.Text: unhandled field %s of %s", k.name, t.Name()))
		}
	}
}

func textSpan(s token.Span) string {
	return fmt.Sprintf("%d:%d:%d-%d:%d:%d", s.Start.Line, s.Start.Col, s.Start.Offset, s.End.Line, s.End.Col, s.End.Offset)
}

// textQuote quotes s byte by byte like the token dump (lexer.Dump).
func textQuote(s string) string {
	const hex = "0123456789abcdef"
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"' || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c >= 0x20 && c < 0x7f:
			b.WriteByte(c)
		default:
			b.WriteString(`\x`)
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&0xf])
		}
	}
	b.WriteByte('"')
	return b.String()
}

// kindName is the Go constant name of an operator token kind, the variant
// name of the Kind enum in selfhost/lexer.
func kindName(k token.Kind) string {
	if n, ok := opNames[k]; ok {
		return n
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

// opNames covers every kind an AST node holds: unary, binary, and
// assignment operators.
var opNames = map[token.Kind]string{
	token.Plus: "Plus", token.Minus: "Minus", token.Star: "Star", token.Slash: "Slash",
	token.Percent: "Percent", token.Shl: "Shl", token.Shr: "Shr", token.Amp: "Amp",
	token.Pipe: "Pipe", token.Caret: "Caret", token.Tilde: "Tilde", token.Bang: "Bang",
	token.AndAnd: "AndAnd", token.OrOr: "OrOr", token.Coalesce: "Coalesce",
	token.Eq: "Eq", token.NotEq: "NotEq", token.Lt: "Lt", token.LtEq: "LtEq", token.Gt: "Gt", token.GtEq: "GtEq",
	token.Assign: "Assign", token.PlusAssign: "PlusAssign", token.MinusAssign: "MinusAssign",
	token.StarAssign: "StarAssign", token.SlashAssign: "SlashAssign", token.PercentAssign: "PercentAssign",
	token.DotDot: "DotDot", token.DotDotEq: "DotDotEq",
}
