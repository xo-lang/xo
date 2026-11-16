package rt

import (
	"fmt"
	"strings"
	"testing"
)

// Ported from pgx v5.12.0 named_args_test.go (TestNamedArgsRewriteQuery,
// TestNamedArgsDollarQuotedStrings, TestStrictNamedArgsRewriteQuery),
// Copyright (c) 2013-2021 Jack Christensen, MIT License; the same cases
// as std/native/sql_test.xo's, for the Go backend.

func namedArgs(kv ...any) Map[string, Sql_SqlValue] {
	var m Map[string, Sql_SqlValue]
	for i := 0; i < len(kv); i += 2 {
		var v Sql_SqlValue
		switch x := kv[i+1].(type) {
		case int:
			v = Sql_SqlValue{Tag: 2, P: Sql_SqlValue_Integer{F_n: int64(x)}}
		case string:
			v = Sql_SqlValue{Tag: 3, P: Sql_SqlValue_Text{F_s: x}}
		}
		Map_put(nil, &m, kv[i].(string), v)
	}
	return m
}

func namedRewrite(q string, args Map[string, Sql_SqlValue], strict bool) string {
	r := Sql_named(nil, q, args, strict)
	if !r.Ok {
		return "error: " + r.E.P.(Sql_SqlErr_Param).F_msg
	}
	var vs []string
	for _, v := range List_Slice(r.V.F1) {
		switch p := v.P.(type) {
		case Sql_SqlValue_Integer:
			vs = append(vs, fmt.Sprint(p.F_n))
		case Sql_SqlValue_Text:
			vs = append(vs, fmt.Sprintf("%q", p.F_s))
		default:
			vs = append(vs, "NULL")
		}
	}
	return r.V.F0 + " | [" + strings.Join(vs, ", ") + "]"
}

func TestSqlNamedRewrite(t *testing.T) {
	id := namedArgs("id", 42)
	for _, tt := range []struct {
		sql  string
		args Map[string, Sql_SqlValue]
		want string
	}{
		{"select * from users where id = @id", id, "select * from users where id = $1 | [42]"},
		{"select * from t where foo < @abc and baz = @def and bar < @abc", namedArgs("abc", 42, "def", 1), "select * from t where foo < $1 and baz = $2 and bar < $1 | [42, 1]"},
		{"select @a::int, @b::text", namedArgs("a", 42, "b", "foo"), `select $1::int, $2::text | [42, "foo"]`},
		{"select @Abc::int, @b_4::text, @_c::int", namedArgs("Abc", 42, "b_4", "foo", "_c", 1), `select $1::int, $2::text, $3::int | [42, "foo", 1]`},
		{"at end @", namedArgs("a", 42), "at end @ | []"},
		{"ignores without valid character after @ foo bar", namedArgs("a", 42), "ignores without valid character after @ foo bar | []"},
		{"name cannot start with number @1 foo bar", namedArgs("a", 42), "name cannot start with number @1 foo bar | []"},
		{`select *, '@foo' as "@bar" from users where id = @id`, id, `select *, '@foo' as "@bar" from users where id = $1 | [42]`},
		{"select * -- @foo\n from users -- @single line comments\n where id = @id;", id, "select * -- @foo\n from users -- @single line comments\n where id = $1; | [42]"},
		{"select * -- C:\\path\\\nfrom users where id = @id", id, "select * -- C:\\path\\\nfrom users where id = $1 | [42]"},
		{"select * /* @multi line\n @comment\n */\n /* /* with @nesting */ */\n from users\n where id = @id;", id, "select * /* @multi line\n @comment\n */\n /* /* with @nesting */ */\n from users\n where id = $1; | [42]"},
		{"extra provided argument", namedArgs("extra", 1), "extra provided argument | []"},
		{"@missing argument", namedArgs(), "$1 argument | [NULL]"},
		{"select * from t where note = 'a\uFFFDb' and id = @id", id, "select * from t where note = 'a\uFFFDb' and id = $1 | [42]"},
		{"select * from t where id = @id -- \uFFFD\nand deleted = false", id, "select * from t where id = $1 -- \uFFFD\nand deleted = false | [42]"},
		{"select * from t where id = @id /* \uFFFD */ and deleted = false", id, "select * from t where id = $1 /* \uFFFD */ and deleted = false | [42]"},
		{"select * from \"a\uFFFDb\" where id = @id", id, "select * from \"a\uFFFDb\" where id = $1 | [42]"},
		{"select * from t where note = e'a\uFFFDb' and id = @id", id, "select * from t where note = e'a\uFFFDb' and id = $1 | [42]"},
		{"select @id\uFFFD, @other", namedArgs("id", 42, "other", 7), "select $1\uFFFD, $2 | [42, 7]"},
		{`select e'it\'s @no', @id`, id, `select e'it\'s @no', $1 | [42]`},
	} {
		if got := namedRewrite(tt.sql, tt.args, false); got != tt.want {
			t.Errorf("named(%q)\n got %q\nwant %q", tt.sql, got, tt.want)
		}
	}
}

func TestSqlNamedDollarQuotedStrings(t *testing.T) {
	for _, tt := range []struct{ sql, want string }{
		{`select $$hello @id$$, @id`, `select $$hello @id$$, $1`},
		{`select $tag_1$@missing$tag_1$, @id`, `select $tag_1$@missing$tag_1$, $1`},
		{`select $世界$@missing$世界$, @id`, `select $世界$@missing$世界$, $1`},
		{"select $\uFFFD$@missing$\uFFFD$, @id", "select $\uFFFD$@missing$\uFFFD$, $1"},
		{`select $$it's " -- /* @missing \ $$, @id`, `select $$it's " -- /* @missing \ $$, $1`},
		{`select $outer$$inner$@missing$inner$ $$ @missing$outer$, @id`, `select $outer$$inner$@missing$inner$ $$ @missing$outer$, $1`},
		{`select $tag$@missing$TAG$@missing$tag$, @id`, `select $tag$@missing$TAG$@missing$tag$, $1`},
		{`select @id, $$@missing$$, $tag$@id$tag$, @id`, `select $1, $$@missing$$, $tag$@id$tag$, $1`},
		{`select @id, $tag$@missing`, `select $1, $tag$@missing`},
		{`select column$tag$, @id`, `select column$tag$, $1`},
		{`select column$$, @id`, `select column$$, $1`},
		{`select example$tag$, @id`, `select example$tag$, $1`},
		{`select 世界$tag$, @id`, `select 世界$tag$, $1`},
		{`select $1$ @id`, `select $1$ $1`},
		{`select $bad-tag$ @id`, `select $bad-tag$ $1`},
		{`select @id, $`, `select $1, $`},
		{`select '$$' , @id`, `select '$$' , $1`},
		{"select /* $$ */ @id -- $tag$", "select /* $$ */ $1 -- $tag$"},
	} {
		for _, strict := range []bool{false, true} {
			if got, want := namedRewrite(tt.sql, namedArgs("id", 42), strict), tt.want+" | [42]"; got != want {
				t.Errorf("named(%q, strict %v)\n got %q\nwant %q", tt.sql, strict, got, want)
			}
		}
	}
	for _, strict := range []bool{false, true} {
		if got := namedRewrite(`select $$hello @world$$`, namedArgs(), strict); got != `select $$hello @world$$ | []` {
			t.Errorf("no phantom arguments (strict %v): %q", strict, got)
		}
	}
	if got := namedRewrite(`select $$@id$$`, namedArgs("id", 42), true); got != "error: named argument id is not used by the query" {
		t.Errorf("strict, argument only in a literal: %q", got)
	}
}

func TestSqlNamedStrict(t *testing.T) {
	for _, tt := range []struct {
		sql  string
		args Map[string, Sql_SqlValue]
		want string
	}{
		{"no arguments", namedArgs(), "no arguments | []"},
		{"@all @matches", namedArgs("all", 1, "matches", 2), "$1 $2 | [1, 2]"},
		{"extra provided argument", namedArgs("extra", 1), "error: named argument extra is not used by the query"},
		{"@missing argument", namedArgs(), "error: named argument @missing has no value"},
	} {
		if got := namedRewrite(tt.sql, tt.args, true); got != tt.want {
			t.Errorf("named(%q, strict)\n got %q\nwant %q", tt.sql, got, tt.want)
		}
	}
}
