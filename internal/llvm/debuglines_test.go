package llvm

import (
	"debug/dwarf"
	"debug/elf"
	"debug/macho"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestDebugLinesLLVM checks that an LLVM debug build has the source lines
// a debugger sets a breakpoint by (main.xo:2 and main.xo:7 of
// testdata/debuglines, main.xo:7 alone at -O1, which inlines add): on
// darwin an -O0 build without LTO gave clang the .ll in its one step
// link, which compiled it to a temporary object, the only file the debug
// map named, and deleted it, so dsymutil found no DWARF and lldb could
// not stop at main.xo:LINE (2026-11-15, 0.51.1 too;
// gotchas/llvm-o0-darwin-no-dwarf.md). Linux keeps the sections in the
// link, so it checks the same rows there.
func TestDebugLinesLLVM(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("darwin and linux only")
	}
	if testing.Short() {
		t.Skip("builds programs")
	}
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found: the LLVM backend tests need it")
	}
	dir := filepath.Join("testdata", "debuglines")
	// -O1 inlines add into main, so only main's own line is kept there
	for _, c := range []struct {
		opt   string
		lines []int
	}{{"O0", []int{2, 7}}, {"O1", []int{7}}} {
		opt := c.opt
		t.Run(opt, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "debuglines")
			res, err := xotool.Build([]string{dir}, xotool.Options{Backend: xotool.BackendLLVM, Flags: []string{"--opt=" + opt}, Out: out, Env: []string{"XO_BIN_CACHE=off"}})
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			if res.Failed {
				t.Fatalf("build failed:\n%s", res.Output)
			}
			lines, err := mainLines(out)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range c.lines {
				if !lines[want] {
					t.Errorf("no line table row for main.xo:%d (a breakpoint there does not resolve); main.xo's rows: %v", want, sortedLines(lines))
				}
			}
		})
	}
}

// mainLines returns the lines of main.xo that the line table of the
// executable at path (or its dSYM bundle) has rows for.
func mainLines(path string) (map[int]bool, error) {
	d, err := openDWARF(path)
	if err != nil {
		return nil, err
	}
	lines := map[int]bool{}
	r := d.Reader()
	for {
		e, err := r.Next()
		if err != nil {
			return nil, err
		}
		if e == nil {
			break
		}
		if e.Tag != dwarf.TagCompileUnit {
			r.SkipChildren()
			continue
		}
		lr, err := d.LineReader(e)
		if err != nil || lr == nil {
			r.SkipChildren()
			continue
		}
		var le dwarf.LineEntry
		for {
			if err := lr.Next(&le); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return nil, err
			}
			if le.File != nil && filepath.Base(le.File.Name) == "main.xo" && le.Line > 0 {
				lines[le.Line] = true
			}
		}
		r.SkipChildren()
	}
	return lines, nil
}

// openDWARF reads the DWARF of an ELF executable, or of a Mach-O one or
// its .dSYM bundle beside it.
func openDWARF(path string) (*dwarf.Data, error) {
	if runtime.GOOS == "linux" {
		f, err := elf.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return f.DWARF()
	}
	cands := []string{path, filepath.Join(path+".dSYM", "Contents", "Resources", "DWARF", filepath.Base(path))}
	var last error
	for _, p := range cands {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		f, err := macho.Open(p)
		if err != nil {
			last = err
			continue
		}
		d, err := f.DWARF()
		f.Close()
		if err == nil {
			return d, nil
		}
		last = err
	}
	if last == nil {
		last = errors.New("no DWARF in " + path + " or its dSYM")
	}
	return nil, last
}

func sortedLines(m map[int]bool) []int {
	var out []int
	for l := range m {
		out = append(out, l)
	}
	sort.Ints(out)
	return out
}
