package rt

import (
	"strings"
	"testing"
	"time"
)

// TestJsonValueNestedDuplicates is the 0616 json review's F3: every level
// with a duplicate key converted its kept values twice, so 40 levels of
// {"a":0,"a":{...}} (600 bytes) took 2^40 steps. Linear now: 64 levels in
// well under a second, the kept values the last ones.
func TestJsonValueNestedDuplicates(t *testing.T) {
	const depth = 64
	text := strings.Repeat(`{"a":0,"a":`, depth) + "1" + strings.Repeat("}", depth)
	start := time.Now()
	r := Json_parse_value(nil, text)
	if el := time.Since(start); el > time.Second {
		t.Fatalf("%d levels of duplicate keys took %v", depth, el)
	}
	if !r.Ok {
		t.Fatalf("parse_value: %v", r)
	}
	v := r.V
	for i := 0; i < depth; i++ {
		o, ok := v.P.(Json_Value_Object)
		if !ok || len(o.F_fields.s) != 1 {
			t.Fatalf("level %d: %#v", i, v)
		}
		v = o.F_fields.s[0].F1
	}
	if n, ok := v.P.(Json_Value_Int); !ok || n.F_n != 1 {
		t.Fatalf("innermost: %#v", v)
	}
}
