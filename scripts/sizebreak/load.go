package main

import (
	"debug/elf"
	"debug/macho"
	"fmt"
	"os"
	"path/filepath"
)

// execFile is a loaded executable: the text section's address and bytes, and
// the symbols that fall inside it.
type execFile struct {
	Name   string
	Format string // "Mach-O darwin/arm64", "ELF linux/arm64"
	Arch   string // "arm64", "amd64"
	Text   TextInfo
	Syms   []Symbol
	Code   []byte // the text section's bytes, for -reach
}

// loadBinary reads a Mach-O or ELF executable.
func loadBinary(path string) (*execFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	var magic [4]byte
	if _, err := f.ReadAt(magic[:], 0); err != nil {
		f.Close()
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	f.Close()
	name := filepath.Base(path)
	switch {
	case magic == [4]byte{0xfe, 0xed, 0xfa, 0xcf}, magic == [4]byte{0xfe, 0xed, 0xfa, 0xce},
		magic == [4]byte{0xcf, 0xfa, 0xed, 0xfe}, magic == [4]byte{0xce, 0xfa, 0xed, 0xfe}:
		return loadMachO(path, name)
	case magic == [4]byte{0x7f, 'E', 'L', 'F'}:
		return loadELF(path, name)
	}
	return nil, fmt.Errorf("%s: not a Mach-O or ELF file", path)
}

// loadMachO reads __TEXT,__text and its symbols. A symbol counts when it is
// in the section and is not a debug stab; symbol tables of Xo's linker carry
// no sizes, so sizebreak derives them from the gaps.
func loadMachO(path, name string) (*execFile, error) {
	f, err := macho.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var text TextInfo
	var code []byte
	for _, s := range f.Sections {
		if s.Seg != "__TEXT" || s.Name != "__text" {
			continue
		}
		text = TextInfo{Addr: s.Addr, Size: s.Size}
		code, err = s.Data()
		if err != nil {
			return nil, fmt.Errorf("%s: %s", path, err)
		}
	}
	if text.Size == 0 {
		return nil, fmt.Errorf("%s: no __TEXT,__text section", path)
	}
	arch := "unknown"
	switch f.Cpu {
	case macho.CpuArm64:
		arch = "arm64"
	case macho.CpuAmd64:
		arch = "amd64"
	}
	return &execFile{
		Name:   name,
		Format: fmt.Sprintf("Mach-O darwin/%s", arch),
		Arch:   arch,
		Text:   text,
		Syms:   machoSyms(f, text),
		Code:   code,
	}, nil
}

func machoSyms(f *macho.File, text TextInfo) []Symbol {
	if f.Symtab == nil {
		return nil
	}
	var out []Symbol
	for _, s := range f.Symtab.Syms {
		if s.Name == "" || s.Type&0xe0 != 0 { // N_STAB
			continue
		}
		if s.Value < text.Addr || s.Value >= text.Addr+text.Size {
			continue
		}
		out = append(out, Symbol{Name: s.Name, Addr: s.Value})
	}
	return out
}

// loadELF reads .text and the function symbols in it. Xo's ELF binaries keep
// their symbol table (the linkers write .symtab), so this is the same
// measurement as the Mach-O one; a stripped binary has no symbols and fails
// the coverage check rather than printing zeros.
func loadELF(path, name string) (*execFile, error) {
	f, err := elf.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var sec *elf.Section
	secIdx := -1
	for i, s := range f.Sections {
		if s.Name == ".text" {
			sec, secIdx = s, i
		}
	}
	if sec == nil || sec.Size == 0 {
		return nil, fmt.Errorf("%s: no .text section", path)
	}
	code, err := sec.Data()
	if err != nil {
		return nil, fmt.Errorf("%s: %s", path, err)
	}
	text := TextInfo{Addr: sec.Addr, Size: sec.Size}
	arch := "unknown"
	switch f.Machine {
	case elf.EM_AARCH64:
		arch = "arm64"
	case elf.EM_X86_64:
		arch = "amd64"
	}
	syms, err := f.Symbols()
	if err != nil {
		return nil, fmt.Errorf("%s: no symbol table (%v)", path, err)
	}
	var out []Symbol
	for _, s := range syms {
		if s.Name == "" || s.Section != elf.SectionIndex(secIdx) {
			continue
		}
		if s.Value < text.Addr || s.Value >= text.Addr+text.Size {
			continue
		}
		out = append(out, Symbol{Name: s.Name, Addr: s.Value})
	}
	return &execFile{
		Name:   name,
		Format: fmt.Sprintf("ELF %s/%s", elfOS(f.OSABI), arch),
		Arch:   arch,
		Text:   text,
		Syms:   out,
		Code:   code,
	}, nil
}

// elfOS names the file's target OS as xo names it.
func elfOS(abi elf.OSABI) string {
	if abi == elf.ELFOSABI_FREEBSD {
		return "freebsd"
	}
	return "linux"
}
