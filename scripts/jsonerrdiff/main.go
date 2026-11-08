// Records encoding/json's decode result for a fixed set of types and inputs: one JSON line per case,
// {"t":TYPE,"in":TEXT,"go":ERROR-OR-"","out":RE-ENCODED-OR-""}.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

type Inner struct {
	N int    `json:"n"`
	S string `json:"s"`
}
type Outer struct {
	Name  string         `json:"name"`
	In    Inner          `json:"in"`
	Items []Inner        `json:"items"`
	M     map[string]int `json:"m"`
	P     *int           `json:"p"`
	B     bool           `json:"b"`
	F     float64        `json:"f"`
	U     uint8          `json:"u"`
}
type Base struct {
	ID int `json:"id"`
}
type Wrapper struct {
	Base
	X string `json:"x"`
}
type Hard struct {
	V Via `json:"v"`
	N int `json:"n"`
}
type Via int

func (v *Via) UnmarshalJSON(b []byte) error {
	if string(b) == "7" {
		*v = 7
		return nil
	}
	return fmt.Errorf("via: bad %s", b)
}

var sample = `{"name":"a","in":{"n":1,"s":"x"},"items":[{"n":2,"s":"y"},{"n":3,"s":"z"}],"m":{"k":4},"p":5,"b":true,"f":1.5,"u":9}`

type tc struct{ t, in string }

func cases() []tc {
	var cs []tc
	add := func(t, in string) { cs = append(cs, tc{t, in}) }
	// truncation at every prefix of the sample
	for i := 0; i < len(sample); i++ {
		add("Outer", sample[:i])
	}
	add("Outer", sample)
	// syntax errors
	for _, s := range []string{`x`, `{`, `{"a"`, `{"a":`, `{"a":1`, `{"a":1,`, `{"a" 1}`, `{a:1}`, `{"a":1,}`, `[1,]`, `[1 2]`, `{"a":1}}`, `{"a":1} x`, `nul`, `nulx`, `tru`, `truex`, `fals`, `01`, `1.`, `1e`, `-`, `+1`, `.5`, `"\x"`, `"\u12"`, "\"a\nb\"", `"abc`, `{"a":"\ud800"}`, `'a'`, ``, ` `, `{"n":1}{"n":2}`, `[`, `]`, `{"a":[}`, "{\"a\":\"\x01\"}", `{"a":1e999}`} {
		add("Outer", s)
	}
	// type errors, one field at a time
	vals := map[string][]string{
		"name": {`1`, `true`, `[]`, `{}`, `null`},
		"in":   {`1`, `"s"`, `[]`, `null`, `{"n":"x"}`, `{"n":1.5}`, `{"s":1}`},
		"items": {`1`, `{}`, `[1]`, `["a"]`, `[{"n":"x"}]`, `[{"n":1},{"n":"y"}]`, `null`, `[null]`},
		"m":    {`1`, `[]`, `{"k":"v"}`, `{"k":1.5}`, `{"k":null}`, `null`},
		"p":    {`"x"`, `1.5`, `null`, `[]`},
		"b":    {`1`, `"true"`, `null`, `[]`},
		"f":    {`"x"`, `true`, `null`, `[]`, `1e999`},
		"u":    {`-1`, `256`, `300`, `1.5`, `"x"`, `null`, `1e2`, `1e0`, `18446744073709551616`},
	}
	for _, f := range []string{"name", "in", "items", "m", "p", "b", "f", "u"} {
		for _, v := range vals[f] {
			add("Outer", `{"`+f+`":`+v+`}`)
		}
	}
	// several errors in one document (which is kept)
	add("Outer", `{"name":1,"u":300}`)
	add("Outer", `{"u":300,"name":1}`)
	add("Outer", `{"in":{"n":"x"},"b":1}`)
	add("Outer", `{"name":1,"zzz":}`)
	add("Outer", `{"name":1`)
	add("Outer", `{"name":1,"b":[`)
	// unknown, duplicate, case-insensitive
	add("Outer", `{"zzz":1}`)
	add("Outer", `{"name":"a","name":"b"}`)
	add("Outer", `{"NAME":"a"}`)
	add("Outer", `{"name":"a","NAME":"b"}`)
	add("Outer", `{"name":"a","NAME":null}`)
	// top-level shapes
	for _, s := range []string{`null`, `1`, `"s"`, `true`, `[]`, `[{}]`} {
		add("Outer", s)
	}
	// embedded
	for _, s := range []string{`{"id":1,"x":"a"}`, `{"id":"1"}`, `{"x":1}`, `{"id":1.5,"x":2}`, `{"ID":1}`, `{"id":null}`} {
		add("Wrapper", s)
	}
	// via (UnmarshalJSON) error order
	for _, s := range []string{`{"v":7,"n":1}`, `{"v":8,"n":1}`, `{"v":8,"n":"x"}`, `{"n":"x","v":8}`, `{"n":"x","v":7}`, `{"v":"x","n":"y"}`, `{"v":null}`} {
		add("Hard", s)
	}
	return cs
}

func decode(t, in string) (any, error) {
	switch t {
	case "Outer":
		var v Outer
		return &v, json.Unmarshal([]byte(in), &v)
	case "Wrapper":
		var v Wrapper
		return &v, json.Unmarshal([]byte(in), &v)
	case "Hard":
		var v Hard
		return &v, json.Unmarshal([]byte(in), &v)
	}
	panic(t)
}

func main() {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	for _, c := range cases() {
		v, err := decode(c.t, c.in)
		e := ""
		if err != nil {
			e = err.Error()
		}
		var b bytes.Buffer
		e2 := json.NewEncoder(&b)
		e2.SetEscapeHTML(false)
		_ = e2.Encode(v)
		enc.Encode(map[string]string{"t": c.t, "in": c.in, "go": e, "out": string(bytes.TrimSpace(b.Bytes()))})
	}
}
