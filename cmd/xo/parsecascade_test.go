package main

import (
	"strings"
	"testing"
)

// TestParamTypeParseNoCascade: a parse error in a parameter type (`r: Int
// ! E`, a Result written as a function's result) is the one error: the
// rest of the signature is not read as `() ! E`, and the function's call
// sites report nothing (field report 0.26.0: two unrelated errors at the
// call).
func TestParamTypeParseNoCascade(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{
		"app/xo.mod":  "module app\n",
		"app/main.xo": "enum E {\n  Bad\n}\n\nfn get(n: Int) -> Int ! E {\n  if n > 0 { Ok(n) } else { Err(E.Bad) }\n}\n\nfn pick(r: Int ! E) -> Int {\n  r ?? _e => 0\n}\n\nfn main(os: Os) {\n  os.stdio.println(\"${pick(get(0))}\")\n}\n",
	})
	var out, errb strings.Builder
	code := run(t, []string{"check", dir + "/app"}, &out, &errb)
	text := out.String() + errb.String()
	if code != 1 || strings.Count(text, ": error ") != 1 || !strings.Contains(text, "main.xo:9:16: error XO0160") {
		t.Fatalf("want exactly the parameter list's XO0160 (exit 1):\n%s", text)
	}
}
