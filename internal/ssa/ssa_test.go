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
