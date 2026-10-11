package main

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

// TestSSADump checks `xo ssadump` (decision 0150): the build stage by
// default, every stage in build order with --stage=all, positions relative
// to the working directory, --each, and the runs it cannot build.
func TestSSADump(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"prog/main.xo": "fn div(a: Int, b: Int) -> Int { a / b }\n\nfn main(os: Os) {\n  os.stdio.println(\"${div(7, 2)}\")\n}\n",
		"lib/lib.xo":   "pub fn twice(n: Int) -> Int { n * 2 }\n",
		"bad/main.xo":  "fn main(os: Os) {\n  let x: Int = \"no\"\n}\n",
	})
	t.Chdir(dir)
	run1 := func(args ...string) (string, int) {
		var out, errOut bytes.Buffer
		code := run(append([]string{"ssadump"}, args...), &out, &errOut)
		return out.String() + errOut.String(), code
	}

	out, code := run1("prog")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	for _, want := range []string{
		"== stage build\nfunc main(v0: Os) -> () {\n",
		"func div(v0: Int, v1: Int) -> Int {\n",
		"v2: Int = div v0, v1 @ prog/main.xo:1:33\n",
		"call rt.landing",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("build stage lacks %q:\n%s", want, out)
		}
	}

	out, code = run1("--stage=all", "prog")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	var stages []string
	for _, m := range regexp.MustCompile(`(?m)^== stage (\w+)$`).FindAllStringSubmatch(out, -1) {
		stages = append(stages, m[1])
	}
	if got := strings.Join(stages, ","); got != "build,unwind,landings,cleanup,rc" {
		t.Errorf("stages = %s", got)
	}
	if !strings.Contains(out, "release ") {
		t.Errorf("the rc stage has no release:\n%s", out)
	}

	out, code = run1("--each", "prog", "lib", "bad")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	for _, want := range []string{
		"=== ssa prog\n== stage build\n",
		"=== ssa lib\n== errors\nno `main` function in the root module\n",
		"=== ssa bad\n== rejected\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("--each lacks %q:\n%s", want, out)
		}
	}

	if out, code = run1("--stage=nope", "prog"); code != 2 || !strings.Contains(out, "unknown stage") {
		t.Errorf("--stage=nope: exit %d: %s", code, out)
	}
	if out, code = run1("--bench=2", "prog"); code != 0 || !strings.Contains(out, "built 1 runs x 2:") {
		t.Errorf("--bench: exit %d: %s", code, out)
	}
}
