package arm64

import (
	"bytes"
	"debug/dwarf"
	"debug/elf"
	"debug/macho"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestDebugInfoSplitDarwin: --debug-info=split (decision 0355) on the
// host: no __DWARF segment in the program, PROG.dSYM beside it with the
// same UUID, a fault's traceback still names lines (the runtime reads
// the dSYM), and lldb resolves a function through it.
func TestDebugInfoSplitDarwin(t *testing.T) {
	if runtime.GOOS != "darwin" || !native() {
		t.Skip("darwin/arm64 host")
	}
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	bin := filepath.Join(t.TempDir(), "split")
	r, err := xotool.Build([]string{filepath.Join("testdata", "split_debug")}, xotool.Options{Backend: xotool.BackendArm64, Out: bin, Flags: []string{"--debug-info=split"}})
	if err != nil || r.Failed {
		t.Fatalf("build: %v\n%s", err, r.Output)
	}
	f, err := macho.Open(bin)
	if err != nil {
		t.Fatal(err)
	}
	if f.Segment("__DWARF") != nil {
		t.Errorf("the split program has a __DWARF segment")
	}
	uuid := machoUUID(t, f)
	f.Close()
	dw := filepath.Join(bin+".dSYM", "Contents", "Resources", "DWARF", "split")
	d, err := macho.Open(dw)
	if err != nil {
		t.Fatalf("no dSYM: %v", err)
	}
	if got := machoUUID(t, d); !bytes.Equal(got, uuid) {
		t.Errorf("dSYM UUID % x, program's % x", got, uuid)
	}
	if d.Section("__debug_info") == nil {
		t.Errorf("the dSYM has no __debug_info")
	}
	d.Close()
	cmd := testtmp.Guard(exec.Command(bin))
	out, _ := cmd.CombinedOutput()
	if !strings.Contains(string(out), "  main (main.xo:11)") {
		t.Errorf("the fault's traceback names no line through the dSYM:\n%s", out)
	}
	if _, err := exec.LookPath("lldb"); err == nil {
		lo, _ := exec.Command("lldb", "--batch", "-o", "image lookup -n xo_f_main", bin).CombinedOutput()
		if !strings.Contains(string(lo), "main.xo:") {
			t.Errorf("lldb found no line for main through the dSYM:\n%s", lo)
		}
	}
}

func machoUUID(t *testing.T, f *macho.File) []byte {
	t.Helper()
	for _, l := range f.Loads {
		raw := l.Raw()
		if len(raw) >= 24 && f.ByteOrder.Uint32(raw) == 0x1b {
			return raw[8:24]
		}
	}
	t.Fatal("no LC_UUID")
	return nil
}

// TestDebugInfoSplitLinux: a linux/arm64 split build (Xo's ELF linker,
// from any host): no .debug_ sections in the program, a GNU build id
// note and a .gnu_debuglink naming PROG.debug, which has the DWARF and
// the same build id. (Docker runs on 2026-10-23: the runtime's
// traceback, llvm-symbolizer and addr2line named main.xo lines through
// it.)
func TestDebugInfoSplitLinux(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program")
	}
	bin := filepath.Join(t.TempDir(), "split")
	r, err := xotool.Build([]string{filepath.Join("testdata", "split_debug")}, xotool.Options{Backend: xotool.BackendArm64, Targets: "linux/arm64", Out: bin, Flags: []string{"--debug-info=split"}})
	if err != nil || r.Failed {
		t.Fatalf("build: %v\n%s", err, r.Output)
	}
	f, err := elf.Open(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, s := range f.Sections {
		if strings.HasPrefix(s.Name, ".debug_") {
			t.Errorf("the split program has %s", s.Name)
		}
	}
	id := elfSection(t, f, ".note.gnu.build-id")
	link := elfSection(t, f, ".gnu_debuglink")
	if name := string(link[:bytes.IndexByte(link, 0)]); name != "split.debug" {
		t.Errorf(".gnu_debuglink names %q", name)
	}
	d, err := elf.Open(bin + ".debug")
	if err != nil {
		t.Fatalf("no debug file: %v", err)
	}
	defer d.Close()
	if got := elfSection(t, d, ".note.gnu.build-id"); !bytes.Equal(got, id) {
		t.Errorf("debug file build id % x, program's % x", got, id)
	}
	dw, err := d.DWARF()
	if err != nil {
		t.Fatalf("the debug file's DWARF: %v", err)
	}
	found := false
	rd := dw.Reader()
	for e, err := rd.Next(); e != nil && err == nil; e, err = rd.Next() {
		if n, ok := e.Val(dwarf.AttrName).(string); ok && n == "pick" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("the debug file's DWARF has no subprogram pick")
	}
	if _, err := os.Stat(bin + ".dSYM"); err == nil {
		t.Errorf("a linux build wrote a dSYM")
	}
}

func elfSection(t *testing.T, f *elf.File, name string) []byte {
	t.Helper()
	s := f.Section(name)
	if s == nil {
		t.Fatalf("no %s", name)
	}
	b, err := s.Data()
	if err != nil {
		t.Fatal(err)
	}
	return b
}
