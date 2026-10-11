package rt

import "testing"

// TestTomlToJSON pins the JSON form of TOML 1.0 documents (std.md 19,
// decision 0164): keys, strings, numbers, dates, tables, and arrays.
func TestTomlToJSON(t *testing.T) {
	cases := []struct{ in, want string }{
		{``, `{}`},
		{"a = 1\nb = -2\nc = +3\nd = 1_000\n", `{"a":1,"b":-2,"c":3,"d":1000}`},
		{"h = 0xDEAD_beef\no = 0o755\nb = 0b1010\n", `{"h":3735928559,"o":493,"b":10}`},
		{"f = +1.5\ng = -0.0\nh = 1e3\ni = 6.626e-34\nj = 1_000.5\n", `{"f":1.5,"g":-0.0,"h":1e3,"i":6.626e-34,"j":1000.5}`},
		{"t = true\nf = false\n", `{"t":true,"f":false}`},
		{`s = "a\tb\u00E9\U0001F600\"\\"`, `{"s":"a\tbé😀\"\\"}`},
		{`s = 'C:\path\no escapes'`, `{"s":"C:\\path\\no escapes"}`},
		{"s = \"\"\"\nline one\nline two\"\"\"", `{"s":"line one\nline two"}`},
		{"s = \"\"\"\\\n    joined \\\n    words\"\"\"", `{"s":"joined words"}`},
		{"s = '''\nraw \\n stays'''", `{"s":"raw \\n stays"}`},
		{`s = """two quotes "" end"""""`, `{"s":"two quotes \"\" end\"\""}`},
		{"\"quoted key\" = 1\n'lit.key' = 2\nbare-key_1 = 3", `{"quoted key":1,"lit.key":2,"bare-key_1":3}`},
		{"a.b.c = 1\na.b.d = 2\na . e = 3", `{"a":{"b":{"c":1,"d":2},"e":3}}`},
		{"[x.y]\nz = 1\n[x]\nw = 2", `{"x":{"y":{"z":1},"w":2}}`},
		{"[[p]]\nn = 1\n[[p]]\nn = 2\n[p.q]\nm = 3", `{"p":[{"n":1},{"n":2,"q":{"m":3}}]}`},
		{"[[a.b]]\nx = 1\n[[a.b]]\nx = 2", `{"a":{"b":[{"x":1},{"x":2}]}}`},
		{"i = { x = 1, y.z = 'w' }\ne = {}", `{"i":{"x":1,"y":{"z":"w"}},"e":{}}`},
		{"a = [ [1, 2], [\"x\"], [], {k = 1} ]", `{"a":[[1,2],["x"],[],{"k":1}]}`},
		{"a = [\n  1, # one\n  2,\n]", `{"a":[1,2]}`},
		{"d1 = 1979-05-27T07:32:00Z\nd2 = 1979-05-27 07:32:00.999-07:00\nd3 = 1979-05-27t00:32:00z\nd4 = 1979-05-27T07:32:00\nd5 = 1979-05-27\nd6 = 07:32:00.5",
			`{"d1":"1979-05-27T07:32:00Z","d2":"1979-05-27T07:32:00.999-07:00","d3":"1979-05-27T00:32:00Z","d4":"1979-05-27T07:32:00","d5":"1979-05-27","d6":"07:32:00.5"}`},
		{"a = 1 # comment\r\nb = 2\r\n", `{"a":1,"b":2}`},
		{"[a]\nb.c = 1\n[a.b.d]\ne = 2", `{"a":{"b":{"c":1,"d":{"e":2}}}}`},
	}
	for _, c := range cases {
		r := Toml_to_json(nil, c.in)
		if !r.Ok {
			t.Errorf("%q: %s", c.in, r.E.M_display(nil))
			continue
		}
		if r.V != c.want {
			t.Errorf("%q:\n got %s\nwant %s", c.in, r.V, c.want)
		}
	}
}

// TestTomlErrors pins the errors of invalid documents and their lines.
func TestTomlErrors(t *testing.T) {
	cases := []struct{ in, want string }{
		{"a", "toml: line 1: expected = after a key"},
		{"= 1", "toml: line 1: expected a key, found '='"},
		{"a = 1 b = 2", "toml: line 1: expected the end of the line, found 'b'"},
		{"a = 1\n\n a = 2", `toml: line 3: key "a" is already defined`},
		{"a = 01", `toml: line 1: bad value "01"`},
		{"a = 1__0", `toml: line 1: bad value "1__0"`},
		{"a = _1", `toml: line 1: bad value "_1"`},
		{"a = 1.", `toml: line 1: bad value "1."`},
		{"a = .5", `toml: line 1: bad value ".5"`},
		{"a = 0x", `toml: line 1: bad value "0x"`},
		{"a = 99999999999999999999", `toml: line 1: bad value "99999999999999999999"`},
		{"a = nan", "toml: line 1: nan cannot be decoded: JSON has no form for inf and nan"},
		{`a = "\x41"`, `toml: line 1: unknown escape \x`},
		{`a = "\uD800"`, `toml: line 1: bad unicode escape \uD800`},
		{"a = \"x\ny\"", "toml: line 1: string not closed"},
		{"a = '''x", "toml: line 1: multi-line string not closed"},
		{"a = [1, 2", "toml: line 1: array not closed"},
		{"a = [1 2]", "toml: line 1: expected , or ] in an array, found '2'"},
		{"a = {x = 1", "toml: line 1: expected , or } in an inline table"},
		{"a = {x = 1}\n[a]", `toml: line 2: table "a" is an inline table`},
		{"a = {x = 1}\na.y = 2", `toml: line 2: key "a.y" is already defined`},
		{"[a]\n[a]", `toml: line 2: table "a" is already defined`},
		{"[a.b]\n[a]\nb = 1", `toml: line 3: key "b" is already defined`},
		{"a = [1]\n[[a]]", `toml: line 2: key "a" is already an array`},
		{"a = 1\n[a]", `toml: line 2: key "a" is already a value`},
		{"[a\nb = 1", "toml: line 1: expected ] after a table header"},
		{"[[a]\n", "toml: line 1: expected ]] after an array of tables header"},
		{"\"\"\"k\"\"\" = 1", "toml: line 1: a key cannot be a multi-line string"},
		{"a = 1979-05-27T25", `toml: line 1: bad value "1979-05-27T25"`},
		{"# ok\na = \"tab\tok\"\nb = \"bell\a\"", "toml: line 3: control character in a string"},
		{"a = \xff", "toml: line 1: the document is not UTF-8"},
	}
	for _, c := range cases {
		r := Toml_to_json(nil, c.in)
		if r.Ok {
			t.Errorf("%q: accepted as %s, want %s", c.in, r.V, c.want)
			continue
		}
		if got := r.E.M_display(nil); got != c.want {
			t.Errorf("%q:\n got %s\nwant %s", c.in, got, c.want)
		}
	}
}
