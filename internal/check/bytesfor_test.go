package check

import "testing"

// TestBytesFor covers `for b in bytes` (core 3.5, decision 0135).
func TestBytesFor(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "yields UInt8", src: "fn f(data: Bytes) -> UInt8 {\n  var last: UInt8 = 0\n  for b in data {\n    last = b\n  }\n  last\n}\n"},
		{name: "compares with rune literals", src: "fn f(s: Str) -> Bool {\n  for c in s.bytes() {\n    if c < 'a' || c > 'z' {\n      return false\n    }\n  }\n  true\n}\n"},
		{name: "slice", src: "fn f(data: Bytes) -> Int {\n  var n = 0\n  for b in data[1..] {\n    n += Int(b)\n  }\n  n\n}\n"},
		{name: "not an Int", src: "fn f(data: Bytes) -> Int {\n  var n = 0\n  for b in data {\n    n += b\n  }\n  n\n}\n", want: []string{"XO0406"}},
		{name: "no index pairs", src: "fn f(data: Bytes) {\n  for (_i, _b) in data {\n  }\n}\n", want: []string{"XO0302"}},
		{name: "Str still needs runes or bytes", src: "fn f(s: Str) {\n  for _c in s {\n  }\n}\n", want: []string{"XO0406"}},
	})
}
