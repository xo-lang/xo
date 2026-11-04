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
