package rt

import "testing"

// TestJsonDupKeyPath (decision 0725): the value a duplicate key replaced
// decodes first, and its error path names the key as ["KEY"], quoted,
// like the kept value's; it read $.KEY with the key raw, so the Go
// backend and the native ones disagreed (TestJSONTextDiff) and a peer's
// key reached the error text unquoted.
func TestJsonDupKeyPath(t *testing.T) {
	for _, c := range []struct{ text, want string }{
		{`{"a":1,"a":"x"}`, `$["a"]`},
		{`{"a\n":1,"a\n":"x"}`, `$["a\n"]`},
		{`{"a":"x","a":1}`, `$["a"]`},
	} {
		x, perr := jsonParse([]byte(c.text))
		if perr != nil {
			t.Fatalf("%s: parse: %v", c.text, perr.F_msg)
		}
		var m Map[string, string]
		err := JsonDec(nil, &m, x, JsonRoot)
		if err == nil {
			t.Fatalf("%s: decoded with no error", c.text)
		}
		if err.F_path != c.want {
			t.Errorf("%s: path %q, want %q", c.text, err.F_path, c.want)
		}
	}
}
