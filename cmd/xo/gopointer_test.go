package main

import (
	"bytes"
	"strings"
	"testing"
)

// The h1 hashes of testdata/goproxy's synthetic example.com/opt v1.0.0.
const (
	optSum    = "h1:QjHoP4+PzsIB3pucWeFrBt5kalFr+P+LLCXA9jKggmU="
	optModSum = "h1:/jrpwqiIF9qM8IzT7TMSinOOOPEvSZd7cm5bhz2I77k="
)

// TestUseGoBasicPointers (decision 0419): a Go pointer to a basic type is
// `T?`, a copy at the boundary: None is nil, Some(v) a pointer to a fresh
// v, and a write Go makes later is seen by reading the field again.
func TestUseGoBasicPointers(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs go list and go build")
	}
	useGoProxy(t)
	dir := writeFiles(t, map[string]string{
		"xo.mod": "module op\n\nrequire go example.com/opt v1.0.0\n",
		"xo.lock": "go example.com/opt v1.0.0 " + optSum + "\n" +
			"go example.com/opt v1.0.0/go.mod " + optModSum + "\n",
		"main.xo": `use go "example.com/opt"

fn main(os: Os) {
  let o = opt.Options.new()
  os.stdio.println(opt.describe(o))
  o.set_name(Some("x"))
  o.set_limit(Some(10))
  os.stdio.println(opt.describe(o))
  o.bump()
  os.stdio.println("${o.limit() ?? -1} ${o.on() ?? false} ${opt.or(None, 7)} ${opt.or(Some(3), 7)} ${opt.ptr(5) ?? 0}")
  o.set_limit(None)
  os.stdio.println(opt.describe(o))
}
`,
	})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"run", "--backend=go", dir}, &out, &errOut); code != 0 {
		t.Fatalf("run: exit %d\n%s%s", code, out.String(), errOut.String())
	}
	want := "name=- limit=- on=-\nname=x limit=10 on=-\n11 false 7 3 5\nname=x limit=- on=-"
	if got := strings.TrimSpace(out.String()); got != want {
		t.Errorf("output\n%s\nwant\n%s", got, want)
	}
}
