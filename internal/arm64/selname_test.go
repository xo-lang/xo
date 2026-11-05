package arm64

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
)

// TestProgramTypeNamedLikeRuntimeHandle builds programs that declare a
// type named Select (an enum, a struct and a newtype) and use `select`,
// and each bare handle name in each type form with every handle in use.
// The compiler interns select's runtime handle under the bare name
// Select (Module.handle), which is not a prelude type, so the program's
// type took that name and rt.select_new returned it: "the runtime call
// rt.select_new returns a Select" on arm64 and LLVM (aws's DynamoDB
// `enum Select`, aws issue 10, 2026-11-04;
// gotchas/program-type-named-like-runtime-handle.md).
func TestProgramTypeNamedLikeRuntimeHandle(t *testing.T) {
	t.Parallel()
	if !native() {
		t.Skip("the host is not arm64")
	}
	const wait = `
fn wait(a: Chan[Int]) -> Int {
  var out = 0
  select {
    v = a.recv() => out = v ?? 99
  }
  out
}
`
	cases := []struct{ name, src, want string }{
		{"enum", `enum Select {
  Count
  All
}

fn pick(s: Select) -> Str {
  match s {
    Count => "count"
    All => "all"
  }
}
` + wait + `
fn main(os: Os) {
  os.stdio.println(pick(Select.Count))
  let a = Chan[Int].new(1)
  a.send(5)
  os.stdio.println("${wait(a)} ${pick(Select.All)}")
}
`, "count\n5 all\n"},
		{"struct", `struct Select {
  n: Int
  s: Str
}
` + wait + `
fn main(os: Os) {
  let a = Chan[Int].new(1)
  a.send(5)
  let w = wait(a)
  let s = Select{n: w, s: "x${w}"}
  os.stdio.println("${w} ${s.n} ${s.s}")
}
`, "5 5 x5\n"},
		{"newtype", `type Select struct(Int)
` + wait + `
fn main(os: Os) {
  let a = Chan[Int].new(1)
  a.send(7)
  let s = Select(wait(a))
  os.stdio.println("${s.0}")
}
`, "7\n"},
	}
	// every bare handle name in each type form, declared before and after
	// its uses, in a program that interns every handle (a scope, spawn,
	// select, listen, dial): the type's value and the handles' results
	// must both come out right (Scope, Listener and Conn were safe
	// before as prelude names; the fix lists them too)
	forms := []struct{ kind, decl, mk, show, want string }{
		{"struct", "struct %[1]s {\n  v: Int\n}\n", "%[1]s{v: n}", "x.v", "3 4 40 5 7\n"},
		{"enum", "enum %[1]s {\n  One(v: Int)\n  Two\n}\n", "%[1]s.One(v: n)", "match x {\n    One(v) => v\n    Two => -1\n  }", "3 4 40 5 7\n"},
		{"newtype", "type %[1]s struct(Int)\n", "%[1]s(n)", "x.0", "3 4 40 5 7\n"},
		{"valenum", "enum %[1]s: Str {\n  Count = \"COUNT\"\n  All = \"ALL\"\n  Other(value: Str)\n}\n", "if n > 5 { %[1]s.Other(value: \"seven\") } else { %[1]s.Count }", "x.value().len()", "5 4 40 5 5\n"},
	}
	const body = `fn mk(n: Int) -> %[1]s {
  %[2]s
}

fn show(x: %[1]s) -> Int {
  %[3]s
}
` + wait + `
fn main(os: Os) ! {
  let x = mk(3)
  let a = Chan[Int].new(1)
  a.send(4)
  let w = wait(a)
  let l = os.net.listen("127.0.0.1:0")?
  let addr = l.addr()
  var got = 0
  scope s {
    let t = s.spawn(fn() -> Int { 40 })
    let c = os.net.dial(addr)?
    c.close()
    a.send(5)
    got = t.await
  }
  l.close()
  os.stdio.println("${show(x)} ${w} ${got} ${a.recv() ?? 0} ${show(mk(7))}")
}
`
	for _, n := range []string{"Scope", "Select", "Listener", "Conn"} {
		for _, f := range forms {
			decl := fmt.Sprintf(f.decl, n)
			b := fmt.Sprintf(body, n, fmt.Sprintf(f.mk, n), f.show)
			cases = append(cases,
				struct{ name, src, want string }{n + "-" + f.kind + "-first", decl + "\n" + b, f.want},
				struct{ name, src, want string }{n + "-" + f.kind + "-last", b + "\n" + decl, f.want})
		}
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir, err := testtmp.Dir("xo-selname-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(dir)
			if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module selname\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(c.src), 0o644); err != nil {
				t.Fatal(err)
			}
			res, err := buildRun(t, dir)
			if err != nil {
				t.Fatal(err)
			}
			if res.exit != 0 || res.stdout != c.want {
				t.Errorf("exit %d, stdout %q, want %q\nstderr:\n%s", res.exit, res.stdout, c.want, res.stderr)
			}
		})
	}
}
