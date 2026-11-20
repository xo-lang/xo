package arm64

import (
	"bytes"
	"debug/elf"
	"debug/macho"
	"fmt"
	"os"
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
	return s
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
