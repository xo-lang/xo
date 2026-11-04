package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestPredeclaredErrSuggestsModuleName: `pub enum Err` in a module is
// XO0603 (Err is predeclared), and its note names the module's own error
// type as std names its own (JwtErr): aws_sig's is AwsSigErr (the AWS
// agent's feedback log, 2026-11-04).
func TestPredeclaredErrSuggestsModuleName(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{
		"xo.mod":  "module example.com/aws_sig\n",
		"main.xo": "pub enum Err {\n  Bad\n}\n\nfn main(os: Os) {\n  os.stdio.println(\"x\")\n}\n",
	})
	var out, errOut bytes.Buffer
	code := run(t, []string{"check", dir}, &out, &errOut)
	all := out.String() + errOut.String()
	if code == 0 || !strings.Contains(all, "XO0603") || !strings.Contains(all, "`AwsSigErr`") {
		t.Errorf("check: exit %d, want XO0603 with a note naming `AwsSigErr`\n%s", code, all)
	}
}

// TestNotPubMethodFix: calling another module's method that is not `pub`
// is XO0605 with a fix that adds `pub` to the method's declaration, and
// so is a function; a `display` that implements Display is callable
// without it (decision 0496 (c), the AWS agent's case, 2026-11-04), one
// with another signature is not.
func TestNotPubMethodFix(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{
		"xo.mod":     "module example.com/app\n",
		"main.xo":    "use example.com/app/lib\n\nfn main(os: Os) {\n  let p = lib.Pt{x: 1}\n  os.stdio.println(p.label())\n  os.stdio.println(p.display())\n  os.stdio.println(lib.Q{}.display(1))\n  os.stdio.println(\"${lib.twice(2)}\")\n}\n",
		"lib/lib.xo": "pub struct Pt {\n  pub x: Int\n}\n\n  fn (p: Pt) label() -> Str {\n  \"${p.x}\"\n}\n\nfn (p: Pt) display() -> Str {\n  \"${p.x}\"\n}\n\npub struct Q {}\n\nfn (q: Q) display(n: Int) -> Str {\n  \"${n}\"\n}\n\nfn twice(n: Int) -> Int { n * 2 }\n",
	})
	setenv(t, "XO_CHK_CACHE", "off")
	var out, errOut bytes.Buffer
	code := run(t, []string{"check", "--json", dir}, &out, &errOut)
	all := out.String() + errOut.String()
	if code == 0 || strings.Count(all, "XO0605") != 3 {
		t.Fatalf("check: exit %d, want three XO0605 (label, Q's display, twice; not Pt's display)\n%s", code, all)
	}
	for _, want := range []string{"make `label` pub in", "make `display` pub in", "make `twice` pub in", `"replace": "pub "`, `"col": 3`} {
		if !strings.Contains(all, want) {
			t.Errorf("no %q in\n%s", want, all)
		}
	}
}
