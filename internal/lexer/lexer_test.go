package lexer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/token"
)

// render lists tokens as "kind:text" (or just the text for operators and
// keywords), with ";" for newline tokens. Values are shown in braces when
// they differ from the text.
func render(toks []token.Token) string {
	var out []string
	for _, t := range toks {
		switch {
		case t.Kind == token.EOF:
			continue
		case t.Kind == token.Newline:
			out = append(out, ";")
		case t.Kind == token.String:
			var ps []string
			for _, p := range t.Parts {
				if p.IsExpr {
					ps = append(ps, "${"+p.Text+"}")
				} else {
					ps = append(ps, fmt.Sprintf("%q", p.Text))
				}
			}
			out = append(out, "str["+strings.Join(ps, " ")+"]")
		case t.Kind.IsOperator() || t.Kind.IsKeyword() || t.Kind == token.Hole:
			out = append(out, t.Text)
		case t.Value != "" && t.Value != t.Text:
			out = append(out, fmt.Sprintf("%s:%s{%q}", short(t.Kind), t.Text, t.Value))
		default:
			out = append(out, short(t.Kind)+":"+t.Text)
		}
	}
	return strings.Join(out, " ")
}

func short(k token.Kind) string {
	switch k {
	case token.Ident:
		return "id"
	case token.Int:
		return "int"
	case token.Float:
		return "float"
	case token.Duration:
		return "dur"
	case token.Rune:
		return "rune"
	case token.RawString:
		return "raw"
	case token.Bytes:
		return "bytes"
	case token.ImportPath:
		return "path"
	}
	return k.String()
}

// nonUse returns the keywords except `use`, which makes the lexer read an
// import path.
func nonUse() []string {
	var out []string
	for _, k := range token.Keywords() {
		if k != "use" {
			out = append(out, k)
		}
	}
	return out
}

func lex(src string) (string, []diag.Diagnostic) {
	var ds diag.List
	r := Lex("t.xo", []byte(src), &ds)
	return render(r.Tokens), ds.Items
}

func TestTokens(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{"idents and keywords", "let x = foo", "let id:x = id:foo ;"},
		{"all keywords", strings.Join(nonUse(), " "), strings.Join(nonUse(), " ")},
		{"contextual words are idents", "derive from with after seed result old", "id:derive id:from id:with id:after id:seed id:result id:old ;"},
		{"ints", "42 1_000_000 0xff 0o755 0b1010 0x_FF", `int:42 int:1_000_000{"1000000"} int:0xff{"255"} int:0o755{"493"} int:0b1010{"10"} int:0x_FF{"255"} ;`},
		{"floats", "3.14 1e-9 2.5E+3 1_0.5", `float:3.14 float:1e-9 float:2.5E+3 float:1_0.5{"10.5"} ;`},
		{"durations", "5s 250ms 10us 2m 1h 7ns", `dur:5s{"5000000000"} dur:250ms{"250000000"} dur:10us{"10000"} dur:2m{"120000000000"} dur:1h{"3600000000000"} dur:7ns{"7"} ;`},
		{"range is not a float", "0..10 xs[1..] 0..=9", "int:0 .. int:10 id:xs [ int:1 .. ] int:0 ..= int:9 ;"},
		{"method on int", "1.max(2)", "int:1 . id:max ( int:2 ) ;"},
		{"tuple fields are not floats", "t.0.1 p.1", "id:t . int:0 . int:1 id:p . int:1 ;"},
		{"float after other tokens", "x = 0.5", "id:x = float:0.5 ;"},
		{"runes", `'a' '\n' '\u{1F600}' '\''`, `rune:'a'{"a"} rune:'\n'{"\n"} rune:'\u{1F600}'{"😀"} rune:'\''{"'"} ;`},
		{"hole and question marks", "??? a?? b x?", "??? id:a ?? id:b id:x ? ;"},
		{"operators", "a += 1 << 2 >>= 3 -> => .. ..= #{ } != == <= >= && || ! ~ ^ % &= |=",
			"id:a += int:1 << int:2 >>= int:3 -> => .. ..= #{ } != == <= >= && || ! ~ ^ % &= |="},
		{"semicolon is a terminator", "a; b", "id:a ; id:b ;"},
		{"import path", "use std/http\nuse github.com/acme/rate-limit as rl", "use path:std/http ; use path:github.com/acme/rate-limit as id:rl ;"},
		{"import path with dashed segments", "use github.com/xo-lang/xo-tools/v2_x.y", "use path:github.com/xo-lang/xo-tools/v2_x.y ;"},
		{"bytes", `b"raw bytes\x00\xff"`, `bytes:b"raw bytes\x00\xff"{"raw bytes\x00\xff"} ;`},
		{"raw string", `r"raw: no escapes \d+ ${x}"`, `raw:r"raw: no escapes \d+ ${x}"{"raw: no escapes \\d+ ${x}"} ;`},
		{"raw hash string", `r#"may contain "quotes": {"a": 1}"#`, `raw:r#"may contain "quotes": {"a": 1}"#{"may contain \"quotes\": {\"a\": 1}"} ;`},
		{"raw double hash", `r##"a "# b"##`, `raw:r##"a "# b"##{"a \"# b"} ;`},
		{"r and b as idents", "r b r1 b2", "id:r id:b id:r1 id:b2 ;"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ds := lex(tt.src)
			if len(ds) > 0 {
				t.Fatalf("unexpected diagnostics: %v", ds)
			}
			if got != tt.want && got != tt.want+" ;" {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}

func TestStrings(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{"plain", `"hello"`, `str["hello"] ;`},
		{"empty", `""`, `str[] ;`},
		{"interpolation", `"hello ${name}, you have ${count + 1} items"`, `str["hello " ${name} ", you have " ${count + 1} " items"] ;`},
		{"literal braces", `"GET /users/{id}"`, `str["GET /users/{id}"] ;`},
		{"escaped quote and brace", `"{\"a\": 1}"`, `str["{\"a\": 1}"] ;`},
		{"dollar escape", `"price: \${x}"`, `str["price: ${x}"] ;`},
		{"lone dollar", `"cost $5 and $"`, `str["cost $5 and $"] ;`},
		{"nested braces and strings", `"${m["k"]} ${f({"a": 1})} ${"in${x}"}"`, `str[${m["k"]} " " ${f({"a": 1})} " " ${"in${x}"}] ;`},
		{"try in interpolation", `"${env.home()?}/.config"`, `str[${env.home()?} "/.config"] ;`},
		{"escapes", `"a\tb\n\\\u{41}\x41\0"`, `str["a\tb\n\\AA\x00"] ;`},
		{"triple strips indentation", "\"\"\"\n    multi line\n      indented\n    end\n    \"\"\"", `str["multi line\n  indented\nend"] ;`},
		{"triple with interpolation", "\"\"\"\n  a ${x}\n  b\n  \"\"\"", `str["a " ${x} "\nb"] ;`},
		{"triple keeps blank lines", "\"\"\"\n  a\n\n  b\n\"\"\"", `str["a\n\nb"] ;`},
		{"triple single line", `"""one "quoted" line"""`, `str["one \"quoted\" line"] ;`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ds := lex(tt.src)
			if len(ds) > 0 {
				t.Fatalf("unexpected diagnostics: %v", ds)
			}
			if got != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}

func TestInterpolationSpans(t *testing.T) {
	var ds diag.List
	src := "let s = \"a ${b.c} d\"\n"
	r := Lex("t.xo", []byte(src), &ds)
	str := r.Tokens[3]
	if str.Kind != token.String || len(str.Parts) != 3 {
		t.Fatalf("unexpected token %+v", str)
	}
	p := str.Parts[1]
	if got := src[p.Span.Start.Offset:p.Span.End.Offset]; got != "b.c" {
		t.Errorf("expression span covers %q, want %q", got, "b.c")
	}
	if p.Span.Start.Col != 14 {
		t.Errorf("expression col = %d, want 14", p.Span.Start.Col)
	}
	sub := LexRange("t.xo", []byte(src), p.Span.Start, p.Span.End.Offset, &ds)
	if got := render(sub); got != "id:b . id:c" {
		t.Errorf("LexRange = %s", got)
	}
	if sub[2].Span.Start.Col != 16 {
		t.Errorf("sub token col = %d, want 16", sub[2].Span.Start.Col)
	}
}

func TestNewlines(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{"terminates after ident", "a\nb", "id:a ; id:b ;"},
		{"not after operator", "a +\nb", "id:a + id:b ;"},
		{"not after comma or open", "f(\na,\nb,\n)", "id:f ( id:a , id:b , ) ;"},
		{"after close paren", "f()\ng()", "id:f ( ) ; id:g ( ) ;"},
		{"after question", "x?\ny", "id:x ? ; id:y ;"},
		{"after return break continue", "return\nbreak\ncontinue\n", "return ; break ; continue ;"},
		{"after literal", "1\n\"s\"\n'c'\n5s\n???\n", `int:1 ; str["s"] ; rune:'c'{"c"} ; dur:5s{"5000000000"} ; ??? ;`},
		{"blank lines collapse", "a\n\n\nb", "id:a ; id:b ;"},
		{"leading dot continues", "http.json(429, x)\n  .with_header(\"a\", \"b\")\n", `id:http . id:json ( int:429 , id:x ) . id:with_header ( str["a"] , str["b"] ) ;`},
		{"leading dot after comment line", "a\n  // note\n  .b()\n", "id:a . id:b ( ) ;"},
		{"leading range does not continue", "a\n..b", "id:a ; .. id:b ;"},
		{"comment keeps newline", "a // trailing\nb", "id:a ; id:b ;"},
		{"brace line", "}\nelse", "} ; else"},
		{"eof terminator", "x", "id:x ;"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ds := lex(tt.src)
			if len(ds) > 0 {
				t.Fatalf("unexpected diagnostics: %v", ds)
			}
			if got != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}

func TestComments(t *testing.T) {
	var ds diag.List
	r := Lex("t.xo", []byte("// plain\n/// doc one\n/// doc two\n//// not doc\nfn f() {}\n"), &ds)
	if len(r.Comments) != 4 {
		t.Fatalf("got %d comments, want 4", len(r.Comments))
	}
	wantDoc := []bool{false, true, true, false}
	wantText := []string{"plain", "doc one", "doc two", "// not doc"}
	for i, c := range r.Comments {
		if c.Doc != wantDoc[i] || c.Text != wantText[i] {
			t.Errorf("comment %d = %+v", i, c)
		}
	}
}

func TestPositions(t *testing.T) {
	var ds diag.List
	r := Lex("t.xo", []byte("let x =\n  \"é\" + y"), &ds)
	y := r.Tokens[5]
	if y.Text != "y" || y.Span.Start.Line != 2 || y.Span.Start.Col != 10 || y.Span.Start.Offset != 17 || y.Span.End.Col != 11 {
		t.Errorf("y token = %+v (columns count bytes)", y.Span)
	}
	if y.Span.File != "t.xo" {
		t.Errorf("file = %q", y.Span.File)
	}
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name, src string
		codes     []diag.Code
	}{
		{"invalid char", "a @ b", []diag.Code{diag.InvalidChar}},
		{"invalid unicode char", "a é", []diag.Code{diag.InvalidChar}},
		{"lone hash", "#x", []diag.Code{diag.InvalidChar}},
		{"unterminated string", "\"abc\nx", []diag.Code{diag.UnterminatedString}},
		{"unterminated raw", `r#"abc"`, []diag.Code{diag.UnterminatedString}},
		{"unterminated triple", `"""abc`, []diag.Code{diag.UnterminatedString}},
		{"unterminated bytes", `b"abc`, []diag.Code{diag.UnterminatedString}},
		{"empty rune", "''", []diag.Code{diag.InvalidRune}},
		{"long rune", "'ab'", []diag.Code{diag.InvalidRune}},
		{"unterminated rune", "'a", []diag.Code{diag.InvalidRune}},
		{"bad escape", `"\q"`, []diag.Code{diag.InvalidEscape}},
		{"bad unicode escape", `"\u{110000}"`, []diag.Code{diag.InvalidEscape}},
		{"high x escape in string", `"\xff"`, []diag.Code{diag.InvalidEscape}},
		{"bad hex", "0xg", []diag.Code{diag.InvalidDuration}},
		{"empty hex", "0x", []diag.Code{diag.MalformedNumber}},
		{"bad binary digit", "0b102", []diag.Code{diag.MalformedNumber}},
		{"double underscore", "1__000", []diag.Code{diag.MalformedNumber}},
		{"trailing underscore", "1000_", []diag.Code{diag.MalformedNumber}},
		{"overflow", "99999999999999999999999", []diag.Code{diag.MalformedNumber}},
		{"unknown unit", "5sec", []diag.Code{diag.InvalidDuration}},
		{"fractional duration", "1.5s", []diag.Code{diag.InvalidDuration}},
		{"hex duration", "0x10s", []diag.Code{diag.InvalidDuration}},
		{"unterminated interpolation", `"a ${b"`, []diag.Code{diag.UnterminatedInterp}},
		{"empty interpolation", `"a ${ }"`, []diag.Code{diag.EmptyInterp}},
		{"bad import path", "use std//http", []diag.Code{diag.MalformedImportPath}},
		{"several errors", "a @ \"\\q\" 1__0", []diag.Code{diag.InvalidChar, diag.InvalidEscape, diag.MalformedNumber}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ds := lex(tt.src)
			var got []diag.Code
			for _, d := range ds {
				got = append(got, diag.Code(d.Code))
			}
			if fmt.Sprint(got) != fmt.Sprint(tt.codes) {
				t.Errorf("codes = %v, want %v (%v)", got, tt.codes, ds)
			}
		})
	}
}

func TestErrorSpan(t *testing.T) {
	_, ds := lex("x = \"\\q\"")
	if len(ds) != 1 {
		t.Fatalf("got %v", ds)
	}
	sp := ds[0].Span
	if sp.Line != 1 || sp.Col != 6 || sp.EndCol != 8 {
		t.Errorf("escape error span = %+v, want col 6 to 8", sp)
	}
}
