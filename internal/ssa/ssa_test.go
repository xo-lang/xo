package ssa_test

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/ssa"
)

var update = flag.Bool("update", false, "rewrite the .ssa golden files")

// lower checks the program in dir and builds and verifies its SSA.
func lower(t *testing.T, dir string) *ssa.Module {
	t.Helper()
	res, ok, err := build.Check([]string{dir}, false)
	if err != nil || !ok {
		var b strings.Builder
		if res != nil {
			for _, d := range res.Diagnostics {
				b.WriteString(diag.Human(d) + "\n")
			}
		}
		t.Fatalf("check failed: %v\n%s", err, b.String())
	}
	m, err := ssa.Build(res)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := m.Verify(); err != nil {
		t.Fatalf("verify: %v\n%s", err, m)
	}
	return m
}

// withRC runs InsertRC and verifies the result, RC balance included.
func withRC(t *testing.T, m *ssa.Module) {
	t.Helper()
	m.InsertRC()
	if err := m.Verify(); err != nil {
		t.Fatalf("verify after InsertRC: %v\n%s", err, m)
	}
}

func golden(t *testing.T, path, got string) {
	t.Helper()
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden file (run with -update): %v", err)
	}
	if string(want) != got {
		t.Errorf("SSA differs from %s\n--- want\n%s\n--- got\n%s", path, want, got)
	}
}

// TestGolden lowers each program under testdata/golden and compares the
// printed SSA with want.ssa.
func TestGolden(t *testing.T) {
	dirs, err := filepath.Glob(filepath.Join("testdata", "golden", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no golden programs: %v", err)
	}
	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			m := lower(t, dir)
			golden(t, filepath.Join(dir, "want.ssa"), m.String())
			withRC(t, m)
			golden(t, filepath.Join(dir, "want.rc.ssa"), m.String())
		})
	}
}

// TestBench lowers and verifies the LLVM benchmark programs
// (bench/perf/llvm/xo), with goldens in testdata/bench.
func TestBench(t *testing.T) {
	dirs, err := filepath.Glob(filepath.Join("..", "..", "bench", "perf", "llvm", "xo", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no bench programs: %v", err)
	}
	for _, dir := range dirs {
		name := filepath.Base(dir)
		t.Run(name, func(t *testing.T) {
			m := lower(t, dir)
			golden(t, filepath.Join("testdata", "bench", name+".ssa"), m.String())
			withRC(t, m)
			golden(t, filepath.Join("testdata", "bench", name+".rc.ssa"), m.String())
		})
	}
}

// TestUnsupported checks that a construct outside the subset is an error
// naming it and its position.
func TestUnsupported(t *testing.T) {
	res, ok, err := build.Check([]string{filepath.Join("testdata", "unsupported")}, false)
	if err != nil || !ok {
		t.Fatalf("check failed: %v", err)
	}
	_, err = ssa.Build(res)
	if err == nil || !strings.Contains(err.Error(), "main.xo:") || !strings.Contains(err.Error(), "closures") {
		t.Fatalf("got %v, want an error about closures with a position", err)
	}
}

// TestVerifyRejects builds broken functions by hand.
func TestVerifyRejects(t *testing.T) {
	cases := []struct {
		name string
		make func(m *ssa.Module)
		want string
	}{
		{"no terminator", func(m *ssa.Module) {
			f := m.NewFunc("f", nil, []*ssa.Type{m.Builtin(ssa.KInt)})
			f.Blocks[0].Add(ssa.OpConst, m.Builtin(ssa.KInt), int64(1))
		}, "does not end with a terminator"},
		{"type mismatch", func(m *ssa.Module) {
			i, fl := m.Builtin(ssa.KInt), m.Builtin(ssa.KFloat)
			f := m.NewFunc("f", []*ssa.Type{i, fl}, []*ssa.Type{i})
			ps := f.Params()
			s := f.Blocks[0].Add(ssa.OpAdd, i, nil, ps[0], ps[1])
			f.Blocks[0].Add(ssa.OpReturn, nil, nil, s)
		}, "one numeric type"},
		{"edge arity", func(m *ssa.Module) {
			i := m.Builtin(ssa.KInt)
			f := m.NewFunc("f", []*ssa.Type{i}, []*ssa.Type{i})
			b1 := f.NewBlock()
			p := b1.AddParam(i)
			b1.Add(ssa.OpReturn, nil, nil, p)
			f.Blocks[0].Jump(b1)
		}, "0 arguments for 1 parameters"},
		{"dominance", func(m *ssa.Module) {
			i, bt := m.Builtin(ssa.KInt), m.Builtin(ssa.KBool)
			f := m.NewFunc("f", []*ssa.Type{bt}, []*ssa.Type{i})
			yes, no, join := f.NewBlock(), f.NewBlock(), f.NewBlock()
			f.Blocks[0].Branch(f.Params()[0], yes, no)
			c := yes.Add(ssa.OpConst, i, int64(1))
			yes.Jump(join)
			no.Jump(join)
			join.Add(ssa.OpReturn, nil, nil, c)
		}, "does not dominate"},
		{"rc leak at ret", func(m *ssa.Module) {
			st, i := m.Builtin(ssa.KStr), m.Builtin(ssa.KInt)
			f := m.NewFunc("f", []*ssa.Type{st}, []*ssa.Type{i})
			n := f.Blocks[0].Add(ssa.OpCall, i, "Str.len", f.Params()[0])
			f.Blocks[0].Add(ssa.OpReturn, nil, nil, n)
			m.RC = true
		}, "held 1 times at ret"},
		{"rc double release", func(m *ssa.Module) {
			st, i := m.Builtin(ssa.KStr), m.Builtin(ssa.KInt)
			f := m.NewFunc("f", []*ssa.Type{st}, []*ssa.Type{i})
			b := f.Blocks[0]
			n := b.Add(ssa.OpCall, i, "Str.len", f.Params()[0])
			b.Add(ssa.OpRelease, nil, nil, f.Params()[0])
			b.Add(ssa.OpRelease, nil, nil, f.Params()[0])
			b.Add(ssa.OpReturn, nil, nil, n)
			m.RC = true
		}, "which is not held"},
		{"rc use after release", func(m *ssa.Module) {
			st, i := m.Builtin(ssa.KStr), m.Builtin(ssa.KInt)
			f := m.NewFunc("f", []*ssa.Type{st}, []*ssa.Type{i})
			b := f.Blocks[0]
			b.Add(ssa.OpRelease, nil, nil, f.Params()[0])
			n := b.Add(ssa.OpCall, i, "Str.len", f.Params()[0])
			b.Add(ssa.OpReturn, nil, nil, n)
			m.RC = true
		}, "which is not alive"},
		{"rc paths disagree", func(m *ssa.Module) {
			st, bt, u := m.Builtin(ssa.KStr), m.Builtin(ssa.KBool), m.Builtin(ssa.KUnit)
			f := m.NewFunc("f", []*ssa.Type{st, bt}, []*ssa.Type{u})
			yes, no, join := f.NewBlock(), f.NewBlock(), f.NewBlock()
			f.Blocks[0].Branch(f.Params()[1], yes, no)
			yes.Add(ssa.OpRelease, nil, nil, f.Params()[0])
			yes.Jump(join)
			no.Jump(join)
			join.Add(ssa.OpRelease, nil, nil, f.Params()[0])
			join.Add(ssa.OpReturn, nil, nil, join.Add(ssa.OpTuple, u, nil))
			m.RC = true
		}, "another predecessor"},
		{"rc update of a shared list", func(m *ssa.Module) {
			l := m.List(m.Builtin(ssa.KInt))
			i := m.Builtin(ssa.KInt)
			f := m.NewFunc("f", []*ssa.Type{l}, []*ssa.Type{l})
			b := f.Blocks[0]
			x := b.Add(ssa.OpConst, i, int64(1))
			p := b.Add(ssa.OpCall, l, "list.push", f.Params()[0], x)
			b.Add(ssa.OpReturn, nil, nil, p)
			m.RC = true
		}, "neither a reuse result nor provably unique"},
		{"rc of a scalar", func(m *ssa.Module) {
			i := m.Builtin(ssa.KInt)
			f := m.NewFunc("f", []*ssa.Type{i}, []*ssa.Type{i})
			f.Blocks[0].Add(ssa.OpRetain, nil, nil, f.Params()[0])
			f.Blocks[0].Add(ssa.OpReturn, nil, nil, f.Params()[0])
		}, "not on the heap"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := ssa.NewModule()
			c.make(m)
			err := m.Verify()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got %v, want an error containing %q", err, c.want)
			}
		})
	}
}

// TestVerifyRCMutations removes, one at a time, every retain and release
// InsertRC put into the RC golden program and the benchmarks, and
// requires the verifier to reject each mutant.
func TestVerifyRCMutations(t *testing.T) {
	dirs := []string{filepath.Join("testdata", "golden", "rc")}
	bench, _ := filepath.Glob(filepath.Join("..", "..", "bench", "perf", "llvm", "xo", "*"))
	dirs = append(dirs, bench...)
	n := 0
	for _, dir := range dirs {
		m := lower(t, dir)
		withRC(t, m)
		for _, f := range m.Funcs {
			for _, b := range f.Blocks {
				for i := 0; i < len(b.Instrs); i++ {
					v := b.Instrs[i]
					if v.Op != ssa.OpRetain && v.Op != ssa.OpRelease {
						continue
					}
					orig := b.Instrs
					b.Instrs = append(append([]*ssa.Value(nil), orig[:i]...), orig[i+1:]...)
					if err := m.Verify(); err == nil {
						t.Errorf("%s: %s: removing %s in %s still verifies", dir, f.Name, v.LongString(), b.Name())
					}
					b.Instrs = orig
					n++
				}
			}
		}
	}
	if n < 10 {
		t.Fatalf("only %d mutants", n)
	}
}
