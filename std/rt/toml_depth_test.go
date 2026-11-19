package rt

import (
	"strings"
	"testing"
)

// TestTomlDecodeDepth (task #352's hostile inputs, tomlfuzz on the Go
// backend): decode follows json.decode's rules on both backends, so a
// document nested past encoding/json's 10,000 levels is a value error
// ("exceeded max depth"), as the native backend's text path gives; the Go
// backend decoded the tree with no limit, and to_json wrote text that
// json.parse_value refuses.
func TestTomlDecodeDepth(t *testing.T) {
	deep := func(n int) string {
		return "x = " + strings.Repeat("[", n) + strings.Repeat("]", n) + "\n"
	}
	r := Toml_decode[Map[string, Json_Value]](nil, deep(10001), false)
	if r.Ok {
		t.Fatalf("10001 levels decoded")
	}
	if v, ok := r.E.P.(Toml_DecodeErr_Value); !ok || !strings.Contains(v.F_err.F_msg, "exceeded max depth") {
		t.Fatalf("10001 levels: %#v, want a value error, exceeded max depth", r.E)
	}
	if r := Toml_decode[Map[string, Json_Value]](nil, deep(9998), false); !r.Ok {
		t.Fatalf("9998 levels: %#v", r.E)
	}
}
