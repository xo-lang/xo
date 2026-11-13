package main

import (
	"debug/elf"
	"debug/macho"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	// Regions are the code sections when there are more than one: a split
	// link's Mach-O puts the program's own modules in __TEXT,_ptext past
	// std's __text (compiler/macho/link.xo). Text then spans their total
	// size from __text's address. Empty means Text and Code alone.
	Regions []Region
}

// Region is one code section: its name, address, size and bytes.
type Region struct {
	Name string
	Addr uint64
	Size uint64
	Code []byte
}

// regionsOf returns bin's code sections, Text and Code as one when it has
// no Regions.
func regionsOf(bin *execFile) []Region {
	if len(bin.Regions) > 0 {
		return bin.Regions
	}
	return []Region{{Name: "__text", Addr: bin.Text.Addr, Size: bin.Text.Size, Code: bin.Code}}
}

// regionEnd returns the end of the region holding addr, and whether one does.
func regionEnd(rs []Region, addr uint64) (uint64, bool) {
	for _, r := range rs {
		if addr >= r.Addr && addr < r.Addr+r.Size {
			return r.Addr + r.Size, true
		}
	}
	return 0, false
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

// loadMachO reads __TEXT,__text, and a split link's __TEXT,_ptext, and their
// symbols. A symbol counts when it is in one of them and is not a debug
// stab; symbol tables of Xo's linker carry no sizes, so sizebreak derives
// them from the gaps, each capped at its section's end. Measuring __text
// alone left out every module of the program's own in a split link
// (aws-43, 2026-11-13: a gcp Calendar program's 2.02 MB of code was 1.61
// MB of __text and 0.42 MB of _ptext).
func loadMachO(path, name string) (*execFile, error) {
	f, err := macho.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var text TextInfo
	var code []byte
	var regions []Region
	for _, s := range f.Sections {
		if s.Seg != "__TEXT" || (s.Name != "__text" && s.Name != "_ptext") {
			continue
		}
		data, err := s.Data()
		if err != nil {
			return nil, fmt.Errorf("%s: %s", path, err)
		}
		regions = append(regions, Region{Name: s.Name, Addr: s.Addr, Size: s.Size, Code: data})
		if s.Name == "__text" {
			text = TextInfo{Addr: s.Addr, Size: s.Size}
			code = data
		}
	}
	if text.Size == 0 {
		return nil, fmt.Errorf("%s: no __TEXT,__text section", path)
	}
	sort.Slice(regions, func(i, j int) bool { return regions[i].Addr < regions[j].Addr })
	if len(regions) > 1 {
		var total uint64
		for _, r := range regions {
			total += r.Size
		}
		text.Size = total
	} else {
		regions = nil
	}
	arch := "unknown"
	switch f.Cpu {
	case macho.CpuArm64:
		arch = "arm64"
	case macho.CpuAmd64:
		arch = "amd64"
	}
	return &execFile{
		Name:    name,
		Format:  fmt.Sprintf("Mach-O darwin/%s", arch),
		Arch:    arch,
		Text:    text,
		Syms:    machoSyms(f, regionsOf(&execFile{Text: text, Code: code, Regions: regions})),
		Code:    code,
		Regions: regions,
	}, nil
}

func machoSyms(f *macho.File, rs []Region) []Symbol {
	if f.Symtab == nil {
		return nil
	}
	var out []Symbol
	for _, s := range f.Symtab.Syms {
		if s.Name == "" || s.Type&0xe0 != 0 { // N_STAB
			continue
		}
		if _, ok := regionEnd(rs, s.Value); !ok {
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
	// .text, and a split link's .text.xoprog (the program's own code past
	// std's, compiler/macho/elflink.xo O_PTEXT)
	var text TextInfo
	var code []byte
	var regions []Region
	idx := map[elf.SectionIndex]bool{}
	for i, s := range f.Sections {
		if (s.Name != ".text" && s.Name != ".text.xoprog") || s.Size == 0 {
			continue
		}
		data, err := s.Data()
		if err != nil {
			return nil, fmt.Errorf("%s: %s", path, err)
		}
		idx[elf.SectionIndex(i)] = true
		regions = append(regions, Region{Name: s.Name, Addr: s.Addr, Size: s.Size, Code: data})
		if s.Name == ".text" {
			text = TextInfo{Addr: s.Addr, Size: s.Size}
			code = data
		}
	}
	if text.Size == 0 {
		return nil, fmt.Errorf("%s: no .text section", path)
	}
	sort.Slice(regions, func(i, j int) bool { return regions[i].Addr < regions[j].Addr })
	if len(regions) > 1 {
		var total uint64
		for _, r := range regions {
			total += r.Size
		}
		text.Size = total
	} else {
		regions = nil
	}
	rs := regionsOf(&execFile{Text: text, Code: code, Regions: regions})
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
		if s.Name == "" || !idx[s.Section] {
			continue
		}
		if _, ok := regionEnd(rs, s.Value); !ok {
			continue
		}
		out = append(out, Symbol{Name: s.Name, Addr: s.Value})
	}
	return &execFile{
		Name:    name,
		Format:  fmt.Sprintf("ELF %s/%s", elfOS(f.OSABI), arch),
		Arch:    arch,
		Text:    text,
		Syms:    out,
		Code:    code,
		Regions: regions,
	}, nil
}

// elfOS names the file's target OS as xo names it.
func elfOS(abi elf.OSABI) string {
	if abi == elf.ELFOSABI_FREEBSD {
		return "freebsd"
	}
	return "linux"
}
