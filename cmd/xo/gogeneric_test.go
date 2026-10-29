package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The h1 hashes of testdata/goproxy's synthetic example.com/generic v1.0.0.
const (
	genericSum    = "h1:YwomAWZ+QbphTWYWxEPjN7VlBYw6DrMzfkM/JZCbiik="
	genericModSum = "h1:RtyxUSA46QJnLhv0ElLE5vG1AEPLL7MOtfyL89JUK8M="
)

// useGoProxy points the go command at testdata/goproxy with a fresh
// module cache, offline (TestUseGoOfflineProxy's setup).
func useGoProxy(t *testing.T) {
	t.Helper()
	proxy, err := filepath.Abs(filepath.Join("testdata", "goproxy"))
	if err != nil {
		t.Fatal(err)
	}
	modcache := t.TempDir()
	t.Cleanup(func() {
		filepath.WalkDir(modcache, func(p string, d os.DirEntry, err error) error {
			if err == nil {
				os.Chmod(p, 0o755)
			}
			return nil
		})
	})
	for k, v := range map[string]string{
		"GOPROXY": "file://" + filepath.ToSlash(proxy), "GOSUMDB": "off", "GOFLAGS": "",
		"GOMODCACHE": modcache, "GOTOOLCHAIN": "local", "GOPRIVATE": "", "GONOSUMDB": "", "GONOPROXY": "",
		"XO_GOBIND_DIR": "",
	} {
		setenv(t, k, v)
	}
}

func genericModule(t *testing.T, main string) string {
	return writeFiles(t, map[string]string{
		"xo.mod": "module gen\n\nrequire go example.com/generic v1.0.0\n",
		"xo.lock": "go example.com/generic v1.0.0 " + genericSum + "\n" +
			"go example.com/generic v1.0.0/go.mod " + genericModSum + "\n",
		"main.xo": main,
	})
}

// TestUseGoGeneric (decision 0415): generic Go functions called from Xo
// with inferred and explicit type arguments: ~[]E as a List, cmp.Ordered
// on Str and Float, an Xo struct through an `any` type parameter (opaque
// to Go), a type parameter only the result names, and ~map[K]V as a Map.
func TestUseGoGeneric(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs go list and go build")
	}
	useGoProxy(t)
	dir := genericModule(t, `use go "example.com/generic"

struct Point {
  x: Int
  y: Int
}

fn main(os: Os) {
  os.stdio.println("${generic.index([3, 1, 4], 4)} ${generic.index(["a", "b"], "z")}")
  os.stdio.println("${generic.max("apple", "pear")} ${generic.max(2.5, 1.5)}")
  let p = generic.keep(Point{x: 1, y: 2})
  os.stdio.println("${p.x + p.y} ${generic.keep([1, 2, 3]).len()}")
  os.stdio.println("${generic.zero[Int]()} '${generic.zero[Str]()}'")
  os.stdio.println("${generic.count({"a": 1, "b": 2})}")
}
`)
	var out, errOut bytes.Buffer
	if code := run(t, []string{"run", "--backend=go", dir}, &out, &errOut); code != 0 {
		t.Fatalf("run: exit %d\n%s%s", code, out.String(), errOut.String())
	}
	want := "2 -1\npear 2.5\n3 3\n0 ''\n2"
	if got := strings.TrimSpace(out.String()); got != want {
		t.Errorf("output\n%s\nwant\n%s", got, want)
	}
}

// TestUseGoGenericNotComparable: a type argument Go must compare
// (comparable, cmp.Ordered) with no comparable Go value is XO1106 at the
// call, not a Go build failure (decision 0415).
func TestUseGoGenericNotComparable(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs go list")
	}
	useGoProxy(t)
	dir := genericModule(t, `use go "example.com/generic"

fn main(os: Os) {
  os.stdio.println("${generic.index([[1], [2]], [2])}")
  os.stdio.println("${generic.max(true, false)}")
}
`)
	var out, errOut bytes.Buffer
	code := run(t, []string{"check", "--backend=go", dir}, &out, &errOut)
	e := errOut.String()
	if code != 1 || !strings.Contains(e, "`List[Int]` has no comparable Go value") || !strings.Contains(e, "XO1106") {
		t.Fatalf("check: exit %d\n%s%s", code, out.String(), e)
	}
}

// The h1 hashes of testdata/goproxy's synthetic example.com/gentype v1.0.0.
const (
	gentypeSum    = "h1:X2+GsPNSkVi8XdX743XN+FkJO4A0LcVF78+7QX+QAOE="
	gentypeModSum = "h1:jjKSHTGxoPeBbz3sz9jEJ5DKu0Zfi0iDAYpk4mimgkQ="
)

// TestUseGoGenericTypes (decision 0415 stage 2): generic Go types as
// generic handles: a Box[T] made by a generic constructor with its
// methods and field, a Set[K comparable] over Str, and a
// Top[T cmp.Ordered] over Float whose method returns (T, bool).
func TestUseGoGenericTypes(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs go list and go build")
	}
	useGoProxy(t)
	dir := writeFiles(t, map[string]string{
		"xo.mod": "module gt\n\nrequire go example.com/gentype v1.0.0\n",
		"xo.lock": "go example.com/gentype v1.0.0 " + gentypeSum + "\n" +
			"go example.com/gentype v1.0.0/go.mod " + gentypeModSum + "\n",
		"main.xo": `use go "example.com/gentype"

fn main(os: Os) {
  let b = gentype.new_box(41)
  b.set(b.get() + 1)
  os.stdio.println("${b.get()} ${b.val()}")
  let s = gentype.new_set[Str]()
  s.add("a")
  s.add("b")
  s.add("a")
  os.stdio.println("${s.len()} ${s.has("a")} ${s.has("z")}")
  let t = gentype.new_top[Float]()
  t.offer(1.5)
  t.offer(3.25)
  t.offer(2.0)
  let (m, ok) = t.max()
  os.stdio.println("${m} ${ok}")
}
`,
	})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"run", "--backend=go", dir}, &out, &errOut); code != 0 {
		t.Fatalf("run: exit %d\n%s%s", code, out.String(), errOut.String())
	}
	want := "42 42\n2 true false\n3.25 true"
	if got := strings.TrimSpace(out.String()); got != want {
		t.Errorf("output\n%s\nwant\n%s", got, want)
	}
}
