package arm64

import (
	"bytes"
	"debug/elf"
	"debug/macho"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// warmDiff says where two builds of one program differ: the first
// differing offset, the sizes and the section that holds the offset (ELF
// or Mach-O), for TestWarmBuildSequence's "differs from its cold build"
// (task #471, 2026-11-20: one gate in about 130 failed that way on
// derive_skip and the test kept nothing to look at).
func warmDiff(warm, cold []byte) string {
	n := min(len(warm), len(cold))
	off := n
	for i := 0; i < n; i++ {
		if warm[i] != cold[i] {
			off = i
			break
		}
	}
	s := fmt.Sprintf("first difference at offset %#x (sizes %d warm, %d cold)", off, len(warm), len(cold))
	if sec := sectionAt(cold, off); sec != "" {
		s += " in " + sec
	}
	if sym := symbolAt(cold, off); sym != "" {
		s += " at " + sym
	}
	return s
}

// symbolAt names the symbol that holds bin's file offset off, as
// NAME+0xDELTA (the nearest symbol at or below its address in the same
// section), "" when there is none: which function or datum differs, for
// a difference whose binaries were not kept (task #476).
func symbolAt(bin []byte, off int) string {
	o := uint64(off)
	if f, err := elf.NewFile(bytes.NewReader(bin)); err == nil {
		syms, _ := f.Symbols()
		for i, s := range f.Sections {
			if s.Type == elf.SHT_NOBITS || o < s.Offset || o >= s.Offset+s.FileSize {
				continue
			}
			addr := s.Addr + (o - s.Offset)
			best := -1
			for j, y := range syms {
				if int(y.Section) == i && y.Name != "" && y.Value <= addr && (best < 0 || y.Value > syms[best].Value) {
					best = j
				}
			}
			if best >= 0 {
				return fmt.Sprintf("%s+%#x", syms[best].Name, addr-syms[best].Value)
			}
			return ""
		}
		return ""
	}
	if f, err := macho.NewFile(bytes.NewReader(bin)); err == nil && f.Symtab != nil {
		for i, s := range f.Sections {
			if o < uint64(s.Offset) || o >= uint64(s.Offset)+s.Size {
				continue
			}
			addr := s.Addr + (o - uint64(s.Offset))
			best := -1
			for j, y := range f.Symtab.Syms {
				if int(y.Sect) == i+1 && y.Name != "" && y.Value <= addr && (best < 0 || y.Value > f.Symtab.Syms[best].Value) {
					best = j
				}
			}
			if best >= 0 {
				y := f.Symtab.Syms[best]
				return fmt.Sprintf("%s+%#x", y.Name, addr-y.Value)
			}
			return ""
		}
	}
	return ""
}

// sectionAt names the section of bin's file offset off, "" when bin is
// not an ELF or Mach-O file or no section holds it.
func sectionAt(bin []byte, off int) string {
	o := uint64(off)
	if f, err := elf.NewFile(bytes.NewReader(bin)); err == nil {
		for _, s := range f.Sections {
			if s.Type != elf.SHT_NOBITS && o >= s.Offset && o < s.Offset+s.FileSize {
				return s.Name
			}
		}
		return ""
	}
	if f, err := macho.NewFile(bytes.NewReader(bin)); err == nil {
		for _, s := range f.Sections {
			if o >= uint64(s.Offset) && o < uint64(s.Offset)+s.Size {
				return s.Seg + "," + s.Name
			}
		}
	}
	return ""
}

// keepWarmDiff copies both builds to $XO_WARMSEQ_KEEP/NAME (scripts/gate.sh
// sets it to the step's logs), so a rare difference can be disassembled
// after the test removed its work directory; "" when unset or on error.
func keepWarmDiff(name string, warm, cold []byte) string {
	k := os.Getenv("XO_WARMSEQ_KEEP")
	if k == "" {
		return ""
	}
	d := filepath.Join(k, name)
	if os.MkdirAll(d, 0o755) != nil || os.WriteFile(filepath.Join(d, "warm"), warm, 0o755) != nil || os.WriteFile(filepath.Join(d, "cold"), cold, 0o755) != nil {
		return ""
	}
	return "; both kept in " + d
}

func TestWarmDiff(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cold, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	// the test binary's own text section: one byte changed inside it
	name, off := "", 0
	if f, err := elf.NewFile(bytes.NewReader(cold)); err == nil {
		s := f.Section(".text")
		name, off = ".text", int(s.Offset+s.FileSize/2)
	} else if f, err := macho.NewFile(bytes.NewReader(cold)); err == nil {
		s := f.Section("__text")
		name, off = "__TEXT,__text", int(uint64(s.Offset)+s.Size/2)
	} else {
		t.Skip("the test binary is neither ELF nor Mach-O")
	}
	warm := append([]byte(nil), cold...)
	warm[off] ^= 0xff
	want := fmt.Sprintf("first difference at offset %#x (sizes %d warm, %d cold) in %s", off, len(warm), len(cold), name)
	if got := warmDiff(warm, cold); got != want {
		t.Errorf("warmDiff = %q, want %q", got, want)
	}
	// a longer warm build that matches the cold one up to its end
	if got, want := warmDiff(append(cold[:16:16], 1), cold[:16]), "first difference at offset 0x10 (sizes 17 warm, 16 cold)"; got != want {
		t.Errorf("warmDiff = %q, want %q", got, want)
	}
	t.Setenv("XO_WARMSEQ_KEEP", t.TempDir())
	if got := keepWarmDiff("p", warm[:8], cold[:8]); got == "" {
		t.Error("keepWarmDiff kept nothing")
	} else if b, err := os.ReadFile(filepath.Join(os.Getenv("XO_WARMSEQ_KEEP"), "p", "warm")); err != nil || !bytes.Equal(b, warm[:8]) {
		t.Errorf("kept warm = %v, %v", b, err)
	}
}

// TestSymbolAt: symbolAt names the function at a file offset of a small
// Go program built here (ELF on linux, Mach-O on darwin; the test binary
// itself has no symbol table, since go test links with -s), 4 bytes into
// it.
func TestSymbolAt(t *testing.T) {
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go tool")
	}
	dir := t.TempDir()
	src := "package main\n\n//go:noinline\nfunc symbolTarget(n int) int { return n*7 + 1 }\n\nfunc main() { println(symbolTarget(3)) }\n"
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "p")
	cmd := exec.Command(gobin, "build", "-o", exe, "main.go")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GO111MODULE=off", "GOFLAGS=")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	bin, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	// debug/macho names it without nm's leading underscore
	const want = "main.symbolTarget"
	off, name := -1, ""
	if f, err := elf.NewFile(bytes.NewReader(bin)); err == nil {
		syms, _ := f.Symbols()
		for _, y := range syms {
			if y.Name == want && int(y.Section) < len(f.Sections) {
				s := f.Sections[y.Section]
				off, name = int(s.Offset+(y.Value-s.Addr))+4, y.Name
				break
			}
		}
	} else if f, err := macho.NewFile(bytes.NewReader(bin)); err == nil && f.Symtab != nil {
		for _, y := range f.Symtab.Syms {
			if (y.Name == want || y.Name == "_"+want) && y.Sect > 0 && int(y.Sect) <= len(f.Sections) {
				s := f.Sections[y.Sect-1]
				off, name = int(uint64(s.Offset)+(y.Value-s.Addr))+4, y.Name
				break
			}
		}
	}
	if off < 0 {
		t.Fatalf("no %s symbol in %s", want, exe)
	}
	if got := symbolAt(bin, off); got != name+"+0x4" {
		t.Errorf("symbolAt(%#x) = %q, want %q", off, got, name+"+0x4")
	}
	if got := symbolAt([]byte("not a binary"), 3); got != "" {
		t.Errorf("symbolAt(not a binary) = %q, want empty", got)
	}
}
